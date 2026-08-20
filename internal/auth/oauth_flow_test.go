package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kurenn/slack-tui/internal/config"
)

// fxAccessServer fakes oauth.v2.access: handler sees the parsed form the
// client sent and returns the HTTP status + JSON body to answer with. Tests
// point accessURL at it via fxPointAccessURL — the seam that makes the real
// HTTP round trip in Exchange/Refresh testable without reaching slack.com.
func fxAccessServer(t *testing.T, handler func(form url.Values) (int, string)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("fake access server: parse form: %v", err)
		}
		status, body := handler(r.Form)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fxPointAccessURL(t *testing.T, u string) {
	t.Helper()
	prev := accessURL
	accessURL = u
	t.Cleanup(func() { accessURL = prev })
}

// fxSimulateBrowser plays the browser's part of the flow: it reads state and
// redirect_uri out of the authorize URL Login built, then GETs the loopback
// callback with the given overrides layered on top of the real state — e.g.
// {"code": {"X"}} for a normal consent, {"error": {"access_denied"}} for a
// denial, or {"state": {"WRONG"}} to simulate a forged/replayed callback.
// Called synchronously from a stubbed browserOpen, in the same goroutine
// Login runs in: the callback handler answers over an independent listener
// goroutine, so this does not deadlock, and the resCh send inside it always
// happens-before Login's blocking select observes it.
func fxSimulateBrowser(t *testing.T, authURL string, overrides url.Values) {
	t.Helper()
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	q := parsed.Query()
	redirect := q.Get("redirect_uri")
	cb := url.Values{"state": {q.Get("state")}}
	for k, v := range overrides {
		cb[k] = v
	}
	resp, err := http.Get(redirect + "?" + cb.Encode())
	if err != nil {
		t.Fatalf("simulated browser GET %s: %v", redirect, err)
	}
	resp.Body.Close()
}

// fxStubBrowser replaces browserOpen with one that hands the captured
// authorize URL straight to fxSimulateBrowser, and restores the real
// (never-invoked-in-tests) implementation afterward.
func fxStubBrowser(t *testing.T, overrides url.Values) {
	t.Helper()
	prev := browserOpen
	browserOpen = func(u string) error {
		fxSimulateBrowser(t, u, overrides)
		return nil
	}
	t.Cleanup(func() { browserOpen = prev })
}

// TestLoginFullPKCEFlow drives Login end-to-end against a fake Slack: a real
// loopback callback server, a stubbed browser completing the consent, and a
// fake oauth.v2.access. The load-bearing assertion is the cross-leg one —
// the code_verifier sent at exchange must S256-hash to the code_challenge
// that was in the authorize URL, computed independently here (not via
// challengeFor) so the test can actually catch a broken PKCE wiring.
func TestLoginFullPKCEFlow(t *testing.T) {
	var gotForm url.Values
	srv := fxAccessServer(t, func(form url.Values) (int, string) {
		gotForm = form
		return http.StatusOK, `{"ok":true,"access_token":"xoxb-bot","team":{"id":"T1","name":"Acme"},"authed_user":{"access_token":"xoxp-user"}}`
	})
	fxPointAccessURL(t, srv.URL)
	fxStubBrowser(t, url.Values{"code": {"FAKECODE"}})

	var authURL string
	// A leftover client_secret (e.g. from an old oauth.json) must never reach
	// either leg of a PKCE flow.
	creds := config.OAuthCreds{ClientID: "CID", ClientSecret: "MUST-NOT-BE-SENT"}
	toks, team, err := Login(context.Background(), creds, func(u string) { authURL = u })
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if toks.User != "xoxp-user" || toks.Bot != "xoxb-bot" {
		t.Errorf("tokens = %+v, want user=xoxp-user bot=xoxb-bot", toks)
	}
	if team.ID != "T1" || team.Name != "Acme" {
		t.Errorf("team = %+v, want T1/Acme", team)
	}
	if authURL == "" {
		t.Fatal("onURL was never called")
	}
	if gotForm == nil {
		t.Fatal("exchange request never reached the fake server")
	}

	parsedAuth, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsedAuth.Query()
	challenge := q.Get("code_challenge")
	redirectAtAuthorize := q.Get("redirect_uri")
	if challenge == "" || redirectAtAuthorize == "" {
		t.Fatalf("authorize URL missing challenge/redirect: %s", authURL)
	}

	// Independent S256 derivation: compute the challenge from the verifier the
	// exchange request actually carried, by hand, rather than reusing
	// challengeFor — this fails if Login ever sends a verifier that doesn't
	// match the challenge it advertised (e.g. regenerating the verifier between
	// building the authorize URL and the exchange).
	verifierSent := gotForm.Get("code_verifier")
	if verifierSent == "" {
		t.Fatal("exchange request carried no code_verifier")
	}
	sum := sha256.Sum256([]byte(verifierSent))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != wantChallenge {
		t.Errorf("code_challenge in authorize URL = %q, but S256(code_verifier sent) = %q", challenge, wantChallenge)
	}

	if gotForm.Get("redirect_uri") != redirectAtAuthorize {
		t.Errorf("redirect_uri at exchange = %q, want the same one from authorize = %q",
			gotForm.Get("redirect_uri"), redirectAtAuthorize)
	}
	if gotForm.Has("client_secret") {
		t.Errorf("client_secret must never be sent, got %v", gotForm)
	}
	if gotForm.Get("client_id") != "CID" {
		t.Errorf("client_id at exchange = %q, want CID", gotForm.Get("client_id"))
	}
}

// TestLoginRejectsStateMismatch guards against accepting a callback that
// didn't originate from the authorize request Login itself made (a forged or
// stale redirect).
func TestLoginRejectsStateMismatch(t *testing.T) {
	fxPointAccessURL(t, "http://unused.invalid") // exchange must never be reached
	fxStubBrowser(t, url.Values{"code": {"FAKECODE"}, "state": {"WRONG-STATE"}})

	_, _, err := Login(context.Background(), config.OAuthCreds{ClientID: "CID"}, nil)
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("err = %v, want a state mismatch error", err)
	}
}

// TestLoginSurfacesDenial covers the user clicking "Deny" at Slack's consent
// screen.
func TestLoginSurfacesDenial(t *testing.T) {
	fxPointAccessURL(t, "http://unused.invalid")
	fxStubBrowser(t, url.Values{"error": {"access_denied"}})

	_, _, err := Login(context.Background(), config.OAuthCreds{ClientID: "CID"}, nil)
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("err = %v, want it to name access_denied", err)
	}
}

// TestLoginContextCancelled: an already-cancelled context must abort waiting
// for the callback rather than hang forever (no code ever arrives because
// nothing plays the browser's part).
func TestLoginContextCancelled(t *testing.T) {
	prev := browserOpen
	browserOpen = func(string) error { return nil } // no callback ever fires
	t.Cleanup(func() { browserOpen = prev })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := Login(ctx, config.OAuthCreds{ClientID: "CID"}, nil)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// TestExchangeSendsExpectedFormAndSecretNever pins the wire contract at the
// HTTP layer (TestExchangeFormNeverSendsSecret in oauth_test.go already pins
// exchangeForm's values in isolation; this drives the real request through
// http.DefaultClient against a server and reads what actually arrived).
func TestExchangeSendsExpectedFormAndSecretNever(t *testing.T) {
	var gotForm url.Values
	srv := fxAccessServer(t, func(form url.Values) (int, string) {
		gotForm = form
		return http.StatusOK, `{"ok":true,"access_token":"xoxb-bot","team":{"id":"T2","name":"Widgets"},"authed_user":{"access_token":"xoxp-user2"}}`
	})
	fxPointAccessURL(t, srv.URL)

	creds := config.OAuthCreds{ClientID: "CID", ClientSecret: "SHH"}
	toks, team, err := Exchange(context.Background(), creds, "AUTHCODE", "VERIF123", "http://localhost:9899/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if toks.User != "xoxp-user2" || toks.Bot != "xoxb-bot" {
		t.Errorf("tokens = %+v", toks)
	}
	if team.ID != "T2" {
		t.Errorf("team = %+v", team)
	}
	if gotForm.Get("client_id") != "CID" || gotForm.Get("code") != "AUTHCODE" ||
		gotForm.Get("redirect_uri") != "http://localhost:9899/callback" || gotForm.Get("code_verifier") != "VERIF123" {
		t.Errorf("form = %v, missing an expected field", gotForm)
	}
	if gotForm.Has("client_secret") {
		t.Errorf("client_secret leaked onto the wire: %v", gotForm)
	}
}

func TestExchangeErrorPaths(t *testing.T) {
	creds := config.OAuthCreds{ClientID: "CID"}

	t.Run("ok false", func(t *testing.T) {
		srv := fxAccessServer(t, func(url.Values) (int, string) {
			return http.StatusOK, `{"ok":false,"error":"invalid_code"}`
		})
		fxPointAccessURL(t, srv.URL)
		_, _, err := Exchange(context.Background(), creds, "C", "V", "R")
		if err == nil || !strings.Contains(err.Error(), "invalid_code") {
			t.Errorf("err = %v, want it to name invalid_code", err)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		srv := fxAccessServer(t, func(url.Values) (int, string) {
			return http.StatusOK, `{"ok": true, "access_token": ` // truncated
		})
		fxPointAccessURL(t, srv.URL)
		_, _, err := Exchange(context.Background(), creds, "C", "V", "R")
		if err == nil {
			t.Error("expected a JSON decode error, got nil")
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := fxAccessServer(t, func(url.Values) (int, string) {
			return http.StatusServiceUnavailable, `<html>Service Unavailable</html>`
		})
		fxPointAccessURL(t, srv.URL)
		_, _, err := Exchange(context.Background(), creds, "C", "V", "R")
		// Exchange doesn't special-case the status code — it decodes whatever
		// body came back. An HTML error page is not valid JSON, so this must
		// fail rather than silently return a zero-value token pair.
		if err == nil {
			t.Error("expected an error decoding a non-JSON error page, got nil")
		}
	})

	t.Run("missing user token", func(t *testing.T) {
		srv := fxAccessServer(t, func(url.Values) (int, string) {
			return http.StatusOK, `{"ok":true,"access_token":"xoxb-bot-only","team":{"id":"T1"}}`
		})
		fxPointAccessURL(t, srv.URL)
		_, _, err := Exchange(context.Background(), creds, "C", "V", "R")
		if err == nil || !strings.Contains(err.Error(), "no user token") {
			t.Errorf("err = %v, want a no-user-token error", err)
		}
	})
}

// TestListenLoopbackPortExhaustion binds every port in the range so none is
// free, then asserts listenLoopback reports failure rather than silently
// returning a listener on the wrong port (which would make Slack's
// redirect_uri check fail confusingly later, inside the OAuth dance).
func TestListenLoopbackPortExhaustion(t *testing.T) {
	var held []net.Listener
	for port := portFirst; port < portFirst+portCount; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			for _, h := range held {
				h.Close()
			}
			t.Skipf("port %d already in use by something else on this machine: %v", port, err)
		}
		held = append(held, ln)
	}
	defer func() {
		for _, h := range held {
			h.Close()
		}
	}()

	_, _, err := listenLoopback()
	if err == nil {
		t.Fatal("expected an error when every loopback port is busy")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", portFirst)) {
		t.Errorf("error should name the port range, got %q", err.Error())
	}
}

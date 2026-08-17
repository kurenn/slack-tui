// Package auth implements Slack's browser OAuth (sign-in) flow for a local app:
// a loopback HTTP server catches the redirect, then the code is exchanged for
// tokens. The app-level token (xapp) for Socket Mode is NOT issued by OAuth —
// it's a static token from the app's admin page.
//
// Two modes share one code path, picked by whether we hold a client secret:
//
//   - PKCE (public client) — the shipped app. No secret is involved: we send a
//     code_challenge on the way out and the code_verifier on the way back, so
//     the binary carries nothing worth extracting. Slack treats a loopback
//     redirect as a desktop redirect once PKCE is on, and desktop redirects
//     may not request bot scopes — so this mode asks for user scopes only.
//   - Confidential — a user's own app, with client_id + client_secret in
//     oauth.json. Keeps bot scopes, which is the only way to get an xoxb token
//     for Socket Mode.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kurenn/slack-tui/internal/config"
)

const (
	// Loopback ports tried in order, first free one wins. A single fixed port
	// meant sign-in failed outright whenever something else held it (or a
	// previous run hadn't released it yet) — with no way to pick another.
	//
	// Slack matches a redirect_uri against the app's registered Redirect URLs by
	// prefix, but the port is part of that prefix, so EVERY port here must be
	// registered separately. Keep this list and slack-app-manifest.* in sync;
	// TestRedirectURIsMatchManifest enforces it.
	portFirst = 9899
	portCount = 5

	authorizeURL = "https://slack.com/oauth/v2/authorize"
	accessURL    = "https://slack.com/api/oauth.v2.access"
)

// RedirectURIs lists every loopback callback the app may use, in preference
// order — exactly what belongs in the app's Redirect URLs.
func RedirectURIs() []string {
	out := make([]string, portCount)
	for i := range out {
		out[i] = redirectURI(portFirst + i)
	}
	return out
}

func redirectURI(port int) string {
	return fmt.Sprintf("http://localhost:%d/callback", port)
}

// listenLoopback binds the first free port in the range, returning it with the
// redirect URI that matches — the two must agree, since Slack compares the
// redirect_uri sent at authorize time with the one sent at exchange time.
func listenLoopback() (net.Listener, string, error) {
	var lastErr error
	for port := portFirst; port < portFirst+portCount; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, redirectURI(port), nil
		}
		lastErr = err
	}
	return nil, "", fmt.Errorf("no free loopback port in %d–%d for the sign-in callback: %w",
		portFirst, portFirst+portCount-1, lastErr)
}

// UserScopes / BotScopes mirror slack-app-manifest.yaml.
var (
	// The four *:write scopes below are what conversations.mark needs — one per
	// conversation kind. Without groups/im/mpim:write, marking read succeeds for
	// public channels and fails with missing_scope everywhere else, so DMs and
	// private channels stay bold in Slack's own clients forever.
	UserScopes = []string{
		"channels:history", "channels:read", "channels:write", "groups:history",
		"groups:read", "groups:write", "im:history", "im:read", "im:write",
		"mpim:history", "mpim:read", "mpim:write",
		"users:read", "chat:write", "files:read", "files:write", "reactions:read", "reactions:write",
		"users:write", "dnd:write", "users.profile:write", "search:read",
	}
	BotScopes = []string{
		"channels:history", "channels:read", "groups:history", "im:history",
		"mpim:history", "users:read",
	}
)

// AuthorizeURL builds the consent URL for the given app + state. A non-empty
// challenge switches the request to PKCE, which also drops the bot scopes:
// Slack rejects them on desktop (loopback) redirects once PKCE is enabled.
func AuthorizeURL(clientID, state, challenge, redirect string) string {
	v := url.Values{
		"client_id":    {clientID},
		"user_scope":   {strings.Join(UserScopes, ",")},
		"redirect_uri": {redirect},
		"state":        {state},
	}
	if challenge != "" {
		v.Set("code_challenge", challenge)
		v.Set("code_challenge_method", "S256")
	} else {
		v.Set("scope", strings.Join(BotScopes, ","))
	}
	return authorizeURL + "?" + v.Encode()
}

// verifier is a PKCE code_verifier: 32 random bytes, base64url without padding
// (RFC 7636 allows 43–128 chars from the unreserved set; this yields 43).
func verifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// challengeFor derives the S256 code_challenge from a verifier.
func challengeFor(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Login runs the full flow: starts the loopback server, opens the browser, waits
// for the redirect, and exchanges the code for tokens. The bot token is merged
// into the result; the caller keeps any existing app (xapp) token.
//
// onURL, when non-nil, receives the authorization URL actually used — state and
// PKCE challenge included — so callers can offer it as a manual fallback when
// the browser doesn't open. Printing a separately-built URL would fail the
// state check, and under PKCE the challenge too.
func Login(ctx context.Context, creds config.OAuthCreds, onURL func(string)) (config.Tokens, Team, error) {
	state, err := randHex()
	if err != nil {
		return config.Tokens{}, Team{}, err
	}
	var verif, challenge string
	if creds.PKCE() {
		if verif, err = verifier(); err != nil {
			return config.Tokens{}, Team{}, err
		}
		challenge = challengeFor(verif)
	}
	ln, redirect, err := listenLoopback()
	if err != nil {
		return config.Tokens{}, Team{}, err
	}
	defer ln.Close()

	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("error") != "":
			page(w, "Authorization was denied.")
			resCh <- result{err: fmt.Errorf("authorization denied: %s", q.Get("error"))}
		case q.Get("state") != state:
			page(w, "State mismatch — please retry.")
			resCh <- result{err: fmt.Errorf("oauth state mismatch")}
		default:
			page(w, "Signed in. You can close this tab and return to the terminal.")
			resCh <- result{code: q.Get("code")}
		}
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	authURL := AuthorizeURL(creds.ClientID, state, challenge, redirect)
	if onURL != nil {
		onURL(authURL)
	}
	_ = openBrowser(authURL) // non-fatal; callers offer the URL via onURL

	select {
	case <-ctx.Done():
		return config.Tokens{}, Team{}, ctx.Err()
	case res := <-resCh:
		if res.err != nil {
			return config.Tokens{}, Team{}, res.err
		}
		return Exchange(ctx, creds, res.code, verif, redirect)
	}
}

// Exchange swaps an authorization code for tokens via oauth.v2.access. Under
// PKCE the verifier replaces the client secret — Slack rejects the call if both
// are sent, so exactly one of them goes on the wire.
func Exchange(ctx context.Context, creds config.OAuthCreds, code, verifier, redirect string) (config.Tokens, Team, error) {
	form := exchangeForm(creds, code, verifier, redirect)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, accessURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return config.Tokens{}, Team{}, err
	}
	defer resp.Body.Close()
	return parseAccess(resp.Body)
}

// exchangeForm builds the oauth.v2.access body. Exactly one credential goes on
// the wire — the verifier under PKCE, the client secret otherwise.
func exchangeForm(creds config.OAuthCreds, code, verifier, redirect string) url.Values {
	form := url.Values{
		"client_id":    {creds.ClientID},
		"code":         {code},
		"redirect_uri": {redirect},
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	} else {
		form.Set("client_secret", creds.ClientSecret)
	}
	return form
}

// Team identifies the workspace the user just authorized.
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type accessResponse struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error"`
	AccessToken string `json:"access_token"` // bot token (xoxb)
	Team        Team   `json:"team"`
	AuthedUser  struct {
		AccessToken string `json:"access_token"` // user token (xoxp)
		// Present only when Slack issues a rotating token. Enabling PKCE caps
		// refresh tokens at 30 days, and forces rotation outright for custom
		// URI schemes — loopback redirects with rotation off should stay
		// non-expiring, but Slack decides, so we record what we were handed.
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"` // seconds
	} `json:"authed_user"`
}

func parseAccess(r io.Reader) (config.Tokens, Team, error) {
	var out accessResponse
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return config.Tokens{}, Team{}, err
	}
	if !out.OK {
		return config.Tokens{}, Team{}, fmt.Errorf("oauth: %s", out.Error)
	}
	if out.AuthedUser.AccessToken == "" {
		return config.Tokens{}, Team{}, fmt.Errorf("oauth: no user token granted (check user_scope)")
	}
	toks := config.Tokens{
		User:    out.AuthedUser.AccessToken,
		Bot:     out.AccessToken,
		Refresh: out.AuthedUser.RefreshToken,
	}
	if out.AuthedUser.ExpiresIn > 0 {
		toks.ExpiresAt = now().Unix() + out.AuthedUser.ExpiresIn
	}
	return toks, out.Team, nil
}

// now is swapped in tests to keep expiry math deterministic.
var now = time.Now

func randHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

func page(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!doctype html><html><body style="font-family:system-ui;background:#0d1117;color:#e6edf3;display:flex;height:100vh;align-items:center;justify-content:center;margin:0"><div style="text-align:center"><h2>slack-tui</h2><p>%s</p></div></body></html>`, msg)
}

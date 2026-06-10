// Package auth implements Slack's browser OAuth (sign-in) flow for a local app:
// a loopback HTTP server catches the redirect, then the code is exchanged for
// tokens. The app-level token (xapp) for Socket Mode is NOT issued by OAuth —
// it's a static token from the app's admin page.
package auth

import (
	"context"
	"crypto/rand"
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

	"github.com/kurenn/slack-tui/internal/config"
)

const (
	// RedirectURI must be registered in the Slack app (OAuth & Permissions →
	// Redirect URLs). Loopback keeps the whole flow on this machine.
	RedirectURI  = "http://localhost:9899/callback"
	listenAddr   = "127.0.0.1:9899"
	authorizeURL = "https://slack.com/oauth/v2/authorize"
	accessURL    = "https://slack.com/api/oauth.v2.access"
)

// UserScopes / BotScopes mirror slack-app-manifest.yaml.
var (
	UserScopes = []string{
		"channels:history", "channels:read", "channels:write", "groups:history",
		"groups:read", "im:history", "im:read", "mpim:history", "mpim:read",
		"users:read", "chat:write", "reactions:read", "reactions:write",
		"users:write", "dnd:write", "users.profile:write", "search:read",
	}
	BotScopes = []string{
		"channels:history", "channels:read", "groups:history", "im:history",
		"mpim:history", "users:read",
	}
)

// AuthorizeURL builds the consent URL for the given app + state.
func AuthorizeURL(clientID, state string) string {
	return authorizeURL + "?" + url.Values{
		"client_id":    {clientID},
		"scope":        {strings.Join(BotScopes, ",")},
		"user_scope":   {strings.Join(UserScopes, ",")},
		"redirect_uri": {RedirectURI},
		"state":        {state},
	}.Encode()
}

// Login runs the full flow: starts the loopback server, opens the browser, waits
// for the redirect, and exchanges the code for tokens. The bot token is merged
// into the result; the caller keeps any existing app (xapp) token.
func Login(ctx context.Context, creds config.OAuthCreds) (config.Tokens, error) {
	state, err := randHex()
	if err != nil {
		return config.Tokens{}, err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return config.Tokens{}, fmt.Errorf("listen on %s: %w (another sign-in already running?)", listenAddr, err)
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

	authURL := AuthorizeURL(creds.ClientID, state)
	_ = openBrowser(authURL) // non-fatal; the URL is also printed by callers

	select {
	case <-ctx.Done():
		return config.Tokens{}, ctx.Err()
	case res := <-resCh:
		if res.err != nil {
			return config.Tokens{}, res.err
		}
		return Exchange(ctx, creds, res.code)
	}
}

// Exchange swaps an authorization code for tokens via oauth.v2.access.
func Exchange(ctx context.Context, creds config.OAuthCreds, code string) (config.Tokens, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, accessURL, strings.NewReader(url.Values{
		"client_id":     {creds.ClientID},
		"client_secret": {creds.ClientSecret},
		"code":          {code},
		"redirect_uri":  {RedirectURI},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return config.Tokens{}, err
	}
	defer resp.Body.Close()
	return parseAccess(resp.Body)
}

type accessResponse struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error"`
	AccessToken string `json:"access_token"` // bot token (xoxb)
	AuthedUser  struct {
		AccessToken string `json:"access_token"` // user token (xoxp)
	} `json:"authed_user"`
}

func parseAccess(r io.Reader) (config.Tokens, error) {
	var out accessResponse
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return config.Tokens{}, err
	}
	if !out.OK {
		return config.Tokens{}, fmt.Errorf("oauth: %s", out.Error)
	}
	if out.AuthedUser.AccessToken == "" {
		return config.Tokens{}, fmt.Errorf("oauth: no user token granted (check user_scope)")
	}
	return config.Tokens{User: out.AuthedUser.AccessToken, Bot: out.AccessToken}, nil
}

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

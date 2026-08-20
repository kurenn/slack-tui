package doctor

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kurenn/slack-tui/internal/auth"
	"github.com/kurenn/slack-tui/internal/config"
)

// smIsolateConfigDir points config.Dir() at a temp dir for the duration of a
// test, and clears the SLACK_* env vars so a real token or one left behind by
// another test can't leak in. Overriding only HOME is not enough: config.Dir
// checks XDG_CONFIG_HOME first, so on any desktop that sets it the test would
// otherwise read/write the real ~/.config/slack-tui.
func smIsolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, v := range []string{"SLACK_USER_TOKEN", "SLACK_APP_TOKEN", "SLACK_BOT_TOKEN"} {
		t.Setenv(v, "")
	}
}

// smRoundTripFunc adapts a function to http.RoundTripper, so each test can
// script the canned Slack responses doctor's network probes hit.
type smRoundTripFunc func(*http.Request) (*http.Response, error)

func (f smRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// smStubHTTP points the package's httpClient var at a fake transport for the
// duration of the test, so the hardcoded https://slack.com URLs in authTest
// and connectionsOpen never resolve — restored automatically on cleanup.
func smStubHTTP(t *testing.T, fn func(req *http.Request) (*http.Response, error)) {
	t.Helper()
	prev := httpClient
	httpClient = &http.Client{Transport: smRoundTripFunc(fn)}
	t.Cleanup(func() { httpClient = prev })
}

// smJSON builds a canned *http.Response carrying a JSON body and optional headers.
func smJSON(status int, headers map[string]string, body string) *http.Response {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestTokenSource(t *testing.T) {
	tests := []struct {
		name            string
		fileVal, envVal string
		want            tokenSourceKind
	}{
		{"unset", "", "", sourceUnset},
		{"file only", "xoxp-file", "", sourceFile},
		{"env only", "", "xoxp-env", sourceEnv},
		{"env overriding file", "xoxp-file", "xoxp-env", sourceOverriding},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenSource(tc.fileVal, tc.envVal); got != tc.want {
				t.Errorf("tokenSource(%q, %q) = %q, want %q", tc.fileVal, tc.envVal, got, tc.want)
			}
		})
	}
}

func TestMissingScopes(t *testing.T) {
	required := []string{"chat:write", "reactions:write", "search:read"}

	t.Run("none missing", func(t *testing.T) {
		granted := []string{"search:read", "chat:write", "reactions:write", "users:read"}
		if got := missingScopes(granted, required); len(got) != 0 {
			t.Errorf("expected no missing, got %v", got)
		}
	})

	t.Run("some missing, in required order", func(t *testing.T) {
		granted := []string{"chat:write"}
		want := []string{"reactions:write", "search:read"}
		if got := missingScopes(granted, required); !reflect.DeepEqual(got, want) {
			t.Errorf("missingScopes = %v, want %v", got, want)
		}
	})

	t.Run("blank entries ignored", func(t *testing.T) {
		granted := []string{"", " chat:write ", "reactions:write", "search:read"}
		if got := missingScopes(granted, required); len(got) != 0 {
			t.Errorf("expected no missing with trimmed grants, got %v", got)
		}
	})
}

func TestMissingScopeFeatureAnnotation(t *testing.T) {
	// Sanity-check the feature map covers the scopes the spec calls out.
	for scope, want := range map[string]string{
		"reactions:write": "a (react)",
		"search:read":     "s (search)",
		"chat:write":      "sending messages",
	} {
		if got := featureFor[scope]; got != want {
			t.Errorf("featureFor[%q] = %q, want %q", scope, got, want)
		}
	}
	// A scope with no mapping yields empty (caller prints bare line).
	if got := featureFor["channels:read"]; got != "" {
		t.Errorf("featureFor[channels:read] = %q, want empty", got)
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"xoxp-1234567890-abcd", "xoxp-…abcd"},
		{"xapp-1-AAAA-BBBB-wxyz", "xapp-…wxyz"},
		{"xoxb-secretlongtoken1234", "xoxb-…1234"},
		{"abc", "abc-…abc"}, // no dash, short: degrades without leaking
	}
	for _, tc := range tests {
		if got := mask(tc.in); got != tc.want {
			t.Errorf("mask(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSplitScopes(t *testing.T) {
	if got := splitScopes(""); got != nil {
		t.Errorf("splitScopes(\"\") = %v, want nil", got)
	}
	want := []string{"chat:write", "search:read"}
	if got := splitScopes("chat:write, search:read"); !reflect.DeepEqual(got, want) {
		t.Errorf("splitScopes = %v, want %v", got, want)
	}
	// Slack actually sends the header comma-separated WITHOUT spaces.
	if got := splitScopes("identify,chat:write,search:read"); !reflect.DeepEqual(got, []string{"identify", "chat:write", "search:read"}) {
		t.Errorf("splitScopes (no spaces) = %v", got)
	}
}

// On Linux with XDG_CONFIG_HOME set, os.UserConfigDir() resolves to the same
// path as config.Dir(), so the current directory was reported as a legacy one.
func TestNoLegacyWarningWhenSamePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	cur, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if legacy := legacyConfigDir(); legacy != cur {
		t.Skipf("platform keeps them distinct (%s vs %s)", legacy, cur)
	}
	if err := os.MkdirAll(cur, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cur, "prefs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, reportConfig)
	if strings.Contains(out, "legacy config also present") {
		t.Errorf("warned about the current dir being legacy:\n%s", out)
	}
}

// captureStdout runs fn with stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = prev
	b, _ := io.ReadAll(r)
	return string(b)
}

// No token anywhere (no file, no env) must fail fast without ever reaching
// the network — the auth section names the exact remediation.
func TestRunNoUserToken(t *testing.T) {
	smIsolateConfigDir(t)
	// Any network call here would mean Run tried to authenticate with an empty
	// token — fail the test rather than let it silently pass or hang.
	smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network call to %s with no token configured", r.URL)
		return nil, nil
	})

	var code int
	out := captureStdout(t, func() { code = Run("v-test") })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "no user token") {
		t.Errorf("expected a 'no user token' line, got:\n%s", out)
	}
}

// A granted-scopes set missing exactly one required scope must name both the
// scope and the feature it breaks — that annotation is what makes the report
// actionable instead of a bare OAuth string.
func TestRunReportsMissingScopeWithFeature(t *testing.T) {
	smIsolateConfigDir(t)
	if err := config.SaveTokens(config.Tokens{User: "xoxp-abc-1234"}); err != nil {
		t.Fatal(err)
	}

	var granted []string
	for _, s := range auth.UserScopes {
		if s != "reactions:write" {
			granted = append(granted, s)
		}
	}
	smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/auth.test":
			return smJSON(200, map[string]string{"X-OAuth-Scopes": strings.Join(granted, ",")},
				`{"ok":true,"team":"Acme","user":"ada","user_id":"U1"}`), nil
		default:
			t.Fatalf("unexpected request to %s", r.URL.Path)
			return nil, nil
		}
	})

	var code int
	out := captureStdout(t, func() { code = Run("v-test") })
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (missing scopes are warnings, not failures)", code)
	}
	if !strings.Contains(out, "missing reactions:write") || !strings.Contains(out, "a (react)") {
		t.Errorf("expected the scope and its feature annotation, got:\n%s", out)
	}
}

// All required scopes granted must print the all-clear line, not a per-scope
// dump — this is the common case and must stay quiet.
func TestRunAllScopesGranted(t *testing.T) {
	smIsolateConfigDir(t)
	if err := config.SaveTokens(config.Tokens{User: "xoxp-abc-1234"}); err != nil {
		t.Fatal(err)
	}
	smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/auth.test":
			return smJSON(200, map[string]string{"X-OAuth-Scopes": strings.Join(auth.UserScopes, ",")},
				`{"ok":true,"team":"Acme","user":"ada","user_id":"U1"}`), nil
		default:
			t.Fatalf("unexpected request to %s", r.URL.Path)
			return nil, nil
		}
	})

	var code int
	out := captureStdout(t, func() { code = Run("v-test") })
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "all required scopes granted") {
		t.Errorf("expected the all-granted line, got:\n%s", out)
	}
	if !strings.Contains(out, "not configured: live channel unread falls back to polling") {
		t.Errorf("expected Socket Mode to report unconfigured (no app/bot token), got:\n%s", out)
	}
}

// auth.test failing at the transport level must exit 1 and say so — the
// report must not claim success when it couldn't verify the token at all.
func TestRunAuthTestHTTPFailure(t *testing.T) {
	smIsolateConfigDir(t)
	if err := config.SaveTokens(config.Tokens{User: "xoxp-abc-1234"}); err != nil {
		t.Fatal(err)
	}
	smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
		return nil, &smNetError{}
	})

	var code int
	out := captureStdout(t, func() { code = Run("v-test") })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "auth.test failed") {
		t.Errorf("expected an auth.test failure line, got:\n%s", out)
	}
	if !strings.Contains(out, "skipped (auth failed)") {
		t.Errorf("expected the scopes section to say it was skipped, got:\n%s", out)
	}
}

// smNetError is a minimal error type standing in for a transport failure
// (DNS/connection refused/etc); doctor only needs *an* error, not its shape.
type smNetError struct{}

func (*smNetError) Error() string { return "stubbed transport failure" }

// An env token shadowing a stored file token is the dangerous "stale env"
// case the tool exists to catch — it must get the "!"-marked overriding row,
// not the quiet env/file rows.
func TestReportTokensEnvOverridingFile(t *testing.T) {
	file := config.Tokens{User: "xoxp-file-1234"}
	env := envValues{user: "xoxp-env-5678"}
	out := captureStdout(t, func() { reportTokens(file, env) })
	if !strings.Contains(out, "! user") || !strings.Contains(out, "env (overriding file!)") {
		t.Errorf("expected the overriding marker on the user row, got:\n%s", out)
	}
	// The masked env value (not the file value) must be what's shown, since
	// env is what's actually in effect.
	if !strings.Contains(out, mask("xoxp-env-5678")) {
		t.Errorf("expected the effective (env) token masked in the row, got:\n%s", out)
	}
}

// reportRotation's four states each need their own distinguishing line —
// this is the logic that tells a user "re-login" apart from "nothing to do".
func TestReportRotationStates(t *testing.T) {
	tests := []struct {
		name   string
		tokens config.Tokens
		want   string
	}{
		{"hand-pasted, no expiry", config.Tokens{User: "x"}, "does not expire (pasted by hand)"},
		{"expiring with no refresh token", config.Tokens{User: "x", ExpiresAt: time.Now().Add(time.Hour).Unix()},
			"has no refresh token — run `slack-tui login`"},
		{"expired", config.Tokens{User: "x", Refresh: "r", ExpiresAt: time.Now().Add(-time.Hour).Unix()},
			"user token expired — it refreshes automatically"},
		{"healthy, refreshes automatically", config.Tokens{User: "x", Refresh: "r", ExpiresAt: time.Now().Add(2 * time.Hour).Unix()},
			"auto-refreshed within"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() { reportRotation(tc.tokens) })
			if !strings.Contains(out, tc.want) {
				t.Errorf("reportRotation(%+v) = %q, want to contain %q", tc.tokens, out, tc.want)
			}
		})
	}
}

// Socket Mode reporting: unconfigured (missing app or bot token), a
// successful probe, and an API-level error each need their own line.
func TestReportSocketModeStates(t *testing.T) {
	t.Run("unconfigured", func(t *testing.T) {
		out := captureStdout(t, func() { reportSocketMode(config.Tokens{User: "x"}) })
		if !strings.Contains(out, "not configured") {
			t.Errorf("got:\n%s", out)
		}
	})

	t.Run("connections.open ok", func(t *testing.T) {
		smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/apps.connections.open" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			return smJSON(200, nil, `{"ok":true}`), nil
		})
		out := captureStdout(t, func() { reportSocketMode(config.Tokens{User: "x", App: "xapp-1", Bot: "xoxb-1"}) })
		if !strings.Contains(out, "Socket Mode available") {
			t.Errorf("got:\n%s", out)
		}
	})

	t.Run("connections.open API error", func(t *testing.T) {
		smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
			return smJSON(200, nil, `{"ok":false,"error":"invalid_auth"}`), nil
		})
		out := captureStdout(t, func() { reportSocketMode(config.Tokens{User: "x", App: "xapp-1", Bot: "xoxb-1"}) })
		if !strings.Contains(out, "invalid_auth") {
			t.Errorf("expected the Slack error surfaced, got:\n%s", out)
		}
	})

	t.Run("connections.open transport error", func(t *testing.T) {
		smStubHTTP(t, func(r *http.Request) (*http.Response, error) {
			return nil, &smNetError{}
		})
		out := captureStdout(t, func() { reportSocketMode(config.Tokens{User: "x", App: "xapp-1", Bot: "xoxb-1"}) })
		if !strings.Contains(out, "stubbed transport failure") {
			t.Errorf("expected the transport error surfaced, got:\n%s", out)
		}
	})
}

// fileExists must distinguish a real file from a directory of the same name
// (os.Stat alone doesn't) and from nothing at all.
func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "d")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !fileExists(file) {
		t.Error("fileExists(file) = false, want true")
	}
	if fileExists(subdir) {
		t.Error("fileExists(dir) = true, want false — a directory is not a file")
	}
	if fileExists(filepath.Join(dir, "missing")) {
		t.Error("fileExists(missing) = true, want false")
	}
}

// dirHasFiles distinguishes "empty", "only subdirectories" (which must count
// as not having files, since it's what makes the legacy-dir warning noise-free
// when only an empty scaffold is left behind) and "has a real file".
func TestDirHasFiles(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		if dirHasFiles(t.TempDir()) {
			t.Error("empty dir should report no files")
		}
	})
	t.Run("only subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		if dirHasFiles(dir) {
			t.Error("a dir containing only subdirectories should report no files")
		}
	})
	t.Run("has a file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !dirHasFiles(dir) {
			t.Error("a dir containing a file should report true")
		}
	})
	t.Run("nonexistent dir", func(t *testing.T) {
		if dirHasFiles(filepath.Join(t.TempDir(), "nope")) {
			t.Error("a nonexistent dir should report no files, not error out")
		}
	})
}

// envTokens must read exactly the three SLACK_* vars doctor classifies —
// nothing more, nothing renamed.
func TestEnvTokens(t *testing.T) {
	t.Setenv("SLACK_USER_TOKEN", "xoxp-e")
	t.Setenv("SLACK_APP_TOKEN", "xapp-e")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-e")
	got := envTokens()
	want := envValues{user: "xoxp-e", app: "xapp-e", bot: "xoxb-e"}
	if got != want {
		t.Errorf("envTokens() = %+v, want %+v", got, want)
	}
}

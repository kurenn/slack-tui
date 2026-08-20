package source

// A hand-rolled fake of the Slack Web API, shared by slack_api_test.go and
// slack_markread_test.go. slack-go v0.25.0 exposes slack.OptionAPIURL, and
// every network call in this package goes through s.api, so a *Slack can be
// pointed at an httptest.Server with zero production changes (verified: no
// raw net/http calls anywhere in slack.go).
//
// The fake stays dumb on purpose (see docs/coverage-plan.md anti-pattern 5):
// handlers serve fixed, test-supplied JSON and record what the client sent.
// They never reimplement Slack's own semantics (pagination cursors, scope
// checks, reaction-toggle behavior, …) — each test configures exactly the
// canned response its scenario needs.
//
// Every new identifier here is prefixed fk to avoid colliding with
// identifiers other coverage units add to this package (see the coverage
// plan's B5 namespace rule).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/slack-go/slack"
)

// fkServer is the fake Slack Web API endpoint.
type fkServer struct {
	t   *testing.T
	mux *http.ServeMux
	srv *httptest.Server

	mu    sync.Mutex
	hits  map[string]int
	forms map[string][]url.Values
}

// fkNewServer starts the fake server. It is closed automatically at test end.
func fkNewServer(t *testing.T) *fkServer {
	t.Helper()
	fk := &fkServer{t: t, mux: http.NewServeMux(), hits: map[string]int{}, forms: map[string][]url.Values{}}
	fk.srv = httptest.NewServer(fk.mux)
	t.Cleanup(fk.srv.Close)
	return fk
}

// on registers a canned responder for a Slack Web API method, e.g.
// fk.on("auth.test", ...). fn writes the HTTP response; it can inspect
// r.PostForm to vary the response (pagination cursors, per-channel replies).
// Every call is recorded (hit count + posted form), in call order.
func (fk *fkServer) on(method string, fn func(w http.ResponseWriter, r *http.Request)) {
	fk.mux.HandleFunc("/"+method, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fk.mu.Lock()
		fk.hits[method]++
		fk.forms[method] = append(fk.forms[method], fkCloneValues(r.PostForm))
		fk.mu.Unlock()
		fn(w, r)
	})
}

// raw registers a responder at an arbitrary path, not recorded like on's
// method handlers — used for the off-endpoint URLs Slack hands back for
// upload (files.getUploadURLExternal) and download (a file's URL).
func (fk *fkServer) raw(path string, fn func(w http.ResponseWriter, r *http.Request)) {
	fk.mux.HandleFunc(path, fn)
}

func fkCloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

// hitCount reports how many times method was called.
func (fk *fkServer) hitCount(method string) int {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.hits[method]
}

// form returns the n-th (0-indexed) posted form for method, or nil if there
// weren't that many calls.
func (fk *fkServer) form(method string, n int) url.Values {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	fs := fk.forms[method]
	if n < 0 || n >= len(fs) {
		return nil
	}
	return fs[n]
}

// lastForm returns the most recently posted form for method.
func (fk *fkServer) lastForm(method string) url.Values {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	fs := fk.forms[method]
	if len(fs) == 0 {
		return nil
	}
	return fs[len(fs)-1]
}

// fkJSON writes v as the canned JSON response body.
func fkJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// fkOK stamps a canned response map as a successful Slack reply.
func fkOK(v map[string]any) map[string]any {
	if v == nil {
		v = map[string]any{}
	}
	v["ok"] = true
	return v
}

// fkErr builds a canned {"ok":false,"error":"..."} Slack failure.
func fkErr(code string) map[string]any {
	return map[string]any{"ok": false, "error": code}
}

// fkClient builds a Slack source wired to fk's fake server. It mirrors
// NewSlack's construction (OptionRetry(3)) so retry/backoff behavior under
// test matches production exactly — a test sending a 429 must set
// Retry-After: 0 or the retrying client really will sleep.
func fkClient(fk *fkServer) *Slack {
	s := NewSlack("xoxp-test-token")
	s.api = slack.New("xoxp-test-token", slack.OptionAPIURL(fk.srv.URL+"/"), slack.OptionRetry(3))
	return s
}

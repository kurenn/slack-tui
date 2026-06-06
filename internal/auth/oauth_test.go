package auth

import (
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	u := AuthorizeURL("CID", "STATE")
	for _, want := range []string{"client_id=CID", "state=STATE", "user_scope=", "redirect_uri=", "users.profile%3Awrite"} {
		if !strings.Contains(u, want) {
			t.Errorf("authorize URL missing %q:\n%s", want, u)
		}
	}
}

func TestParseAccess(t *testing.T) {
	toks, err := parseAccess(strings.NewReader(`{"ok":true,"access_token":"xoxb-bot","authed_user":{"access_token":"xoxp-user"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if toks.User != "xoxp-user" || toks.Bot != "xoxb-bot" {
		t.Errorf("got %+v", toks)
	}
	if _, err := parseAccess(strings.NewReader(`{"ok":false,"error":"invalid_code"}`)); err == nil {
		t.Error("expected error when ok:false")
	}
	if _, err := parseAccess(strings.NewReader(`{"ok":true,"access_token":"xoxb"}`)); err == nil {
		t.Error("expected error when no user token granted")
	}
}

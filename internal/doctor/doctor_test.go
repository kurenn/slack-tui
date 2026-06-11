package doctor

import (
	"reflect"
	"testing"
)

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

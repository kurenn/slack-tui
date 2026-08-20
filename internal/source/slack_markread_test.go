package source

// End-to-end coverage of MarkRead/markErr, mirroring the shape of v0.5.3's
// production bug (commit 75dd588): conversations.mark needs one *:write scope
// per conversation kind, and a token issued before those scopes were
// requested marks public channels fine and returns missing_scope everywhere
// else — silently, if the result is discarded. These tests pin the fix at the
// source layer: the request really is issued for every conversation kind,
// and a missing_scope response comes back as an actionable error rather than
// being swallowed.

import (
	"net/http"
	"strings"
	"testing"
)

// TestMarkReadPerConversationKind: one canned conversations.mark response per
// conversation kind, keyed on the channel ID prefix the way a real Slack
// token would differ per kind (channels:write granted, groups/im/mpim:write
// not). Each kind's request must actually be sent, and a missing_scope
// response must surface as an error, not be swallowed. This is the exact
// shape of the shipped bug: it would have zero test failures if MarkRead
// silently returned nil on error.
func TestMarkReadPerConversationKind(t *testing.T) {
	fk := fkNewServer(t)
	s := fkClient(fk)

	// channels:write only — mirrors the token that shipped the bug.
	fk.on("conversations.mark", func(w http.ResponseWriter, r *http.Request) {
		ch := r.PostForm.Get("channel")
		switch {
		case strings.HasPrefix(ch, "C"): // public channel
			fkJSON(w, fkOK(nil))
		default: // D (im), G (private channel), or mpim — all missing_scope
			fkJSON(w, fkErr("missing_scope"))
		}
	})

	cases := []struct {
		kind    string
		convID  string
		wantErr bool
	}{
		{"public channel", "C0PUBLIC", false},
		{"private channel", "G0PRIVATE", true},
		{"im", "D0IM", true},
		{"mpim", "G0MPIM", true}, // Slack represents mpims as group-shaped IDs too
	}
	for i, c := range cases {
		ts := "1700000000.00000" + string(rune('0'+i))
		err := s.MarkRead(c.convID, ts)
		if (err != nil) != c.wantErr {
			t.Errorf("%s (%s): MarkRead error = %v, wantErr %v", c.kind, c.convID, err, c.wantErr)
			continue
		}
		if c.wantErr && !strings.Contains(err.Error(), "missing a mark-read scope") {
			t.Errorf("%s: error %q should name the missing scope, not swallow it", c.kind, err)
		}
		// The request must actually have been issued for this conversation —
		// the historical bug's damage was that the *result* was discarded, but
		// a version of the bug that never issues the request at all (e.g. an
		// early return for non-public kinds) would be just as broken and must
		// also fail this check.
		got := fk.lastForm("conversations.mark")
		if got.Get("channel") != c.convID || got.Get("ts") != ts {
			t.Errorf("%s: conversations.mark not sent with channel=%s ts=%s, got %v", c.kind, c.convID, ts, got)
		}
	}
	if n := fk.hitCount("conversations.mark"); n != len(cases) {
		t.Errorf("conversations.mark called %d times, want %d (one per conversation kind)", n, len(cases))
	}
}

// TestMarkReadEmptyTSIsNoOp: an empty ts (nothing read yet) must not hit the
// network at all.
func TestMarkReadEmptyTSIsNoOp(t *testing.T) {
	fk := fkNewServer(t)
	s := fkClient(fk)
	fk.on("conversations.mark", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("conversations.mark should not be called for an empty ts")
	})
	if err := s.MarkRead("C1", ""); err != nil {
		t.Fatalf("MarkRead with empty ts = %v, want nil", err)
	}
}

// TestMarkReadGenericErrorNotMisreportedAsScope: a non-scope failure (e.g.
// channel_not_found) must not be mislabeled as the scope-remediation message
// — that would send a user to re-authorize for a problem re-authorizing
// can't fix.
func TestMarkReadGenericErrorNotMisreportedAsScope(t *testing.T) {
	fk := fkNewServer(t)
	s := fkClient(fk)
	fk.on("conversations.mark", func(w http.ResponseWriter, r *http.Request) { fkJSON(w, fkErr("channel_not_found")) })
	err := s.MarkRead("C1", "1700000000.000000")
	if err == nil {
		t.Fatal("MarkRead should surface the channel_not_found error")
	}
	if strings.Contains(err.Error(), "missing a mark-read scope") {
		t.Errorf("channel_not_found should not be reported as a scope problem: %v", err)
	}
	if !strings.Contains(err.Error(), "mark read:") {
		t.Errorf("generic mark failures should be wrapped with context, got %v", err)
	}
}

// TestMarkReadUpdatesCachedMarker: a successful mark must move the cached
// read marker immediately, so the very next unread poll sees the new
// position without waiting on a conversations.info refresh (or the TTL). If
// MarkRead stopped updating the cache, a just-cleared badge would silently
// resurrect until the 5-minute TTL happened to refresh it.
func TestMarkReadUpdatesCachedMarker(t *testing.T) {
	fk := fkNewServer(t)
	s := fkClient(fk)

	fk.on("conversations.mark", func(w http.ResponseWriter, r *http.Request) { fkJSON(w, fkOK(nil)) })
	fk.on("conversations.info", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("conversations.info should not be hit — the cache should already hold a fresh marker after MarkRead")
	})
	var gotOldest string
	fk.on("conversations.history", func(w http.ResponseWriter, r *http.Request) {
		gotOldest = r.PostForm.Get("oldest")
		fkJSON(w, fkOK(map[string]any{"messages": []map[string]any{}}))
	})

	// Seed the cache the way lastReadOf would from a prior fetch, so the
	// no-conversations.info expectation above is meaningful (there's
	// something to fall back on) rather than accidentally testing the
	// no-cache-no-info-call path.
	s.setLastRead("C1", "1699999999.000000")

	if err := s.MarkRead("C1", "1700000123.000000"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if _, err := s.Unread("C1"); err != nil {
		t.Fatalf("Unread: %v", err)
	}
	if gotOldest != "1700000123.000000" {
		t.Errorf("history oldest = %q, want the marker MarkRead just set", gotOldest)
	}
}

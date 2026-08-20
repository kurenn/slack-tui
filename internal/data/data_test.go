package data

import "testing"

// The mock workspace is not test scaffolding — it's the go run . dev backend
// and the substrate every app/component test renders against. These tests
// check *relations* in the fixture (every message author is a real user,
// every message bucket is a real conversation), not particular literal
// values — so they fail exactly when someone edits the sample workspace and
// dangles a reference, the bug class a hand-authored fixture invites.
func TestMockReferentialIntegrity(t *testing.T) {
	ws := Mock()

	convIDs := map[string]bool{}
	for _, c := range ws.Channels {
		convIDs[c.ID] = true
	}
	for _, c := range ws.DMs {
		convIDs[c.ID] = true
	}

	if len(ws.Messages) == 0 {
		t.Fatal("mock workspace has no message buckets at all")
	}
	for convID, msgs := range ws.Messages {
		if !convIDs[convID] {
			t.Errorf("Messages has a bucket for conversation %q, which is not in Channels or DMs", convID)
		}
		for _, m := range msgs {
			if _, ok := ws.Users[m.UserID]; !ok {
				t.Errorf("message %q in %q references unknown user %q", m.ID, convID, m.UserID)
			}
			for _, r := range m.Replies {
				if _, ok := ws.Users[r.UserID]; !ok {
					t.Errorf("reply %q on message %q references unknown user %q", r.ID, m.ID, r.UserID)
				}
			}
		}
	}

	// A DM conversation's UserID must resolve to a real user — it's how the
	// sidebar names the DM and looks up the presence dot.
	for _, c := range ws.DMs {
		if _, ok := ws.Users[c.UserID]; !ok {
			t.Errorf("DM %q references unknown user %q", c.ID, c.UserID)
		}
	}
}

// MeID must point at an actual entry in Users — Me() silently returning the
// zero User would show an empty name/handle in the title bar and composer.
func TestMeResolvesASeededUser(t *testing.T) {
	ws := Mock()
	if _, ok := ws.Users[ws.MeID]; !ok {
		t.Fatalf("MeID %q does not exist in Users", ws.MeID)
	}
	me := ws.Me()
	if me.ID != ws.MeID {
		t.Errorf("Me().ID = %q, want %q", me.ID, ws.MeID)
	}
	if me.Name == "" || me.Handle == "" {
		t.Error("Me() returned a user with an empty name/handle")
	}
}

// Conversation must find both channels and DMs by id, and report "not found"
// rather than a false zero-value hit for an id that doesn't exist.
func TestConversationLookup(t *testing.T) {
	ws := Mock()
	if len(ws.Channels) == 0 || len(ws.DMs) == 0 {
		t.Fatal("fixture needs at least one channel and one DM for this test to mean anything")
	}

	wantChan := ws.Channels[0]
	if got, ok := ws.Conversation(wantChan.ID); !ok || got.ID != wantChan.ID {
		t.Errorf("Conversation(%q) = %+v, %v; want the channel found", wantChan.ID, got, ok)
	}

	wantDM := ws.DMs[0]
	if got, ok := ws.Conversation(wantDM.ID); !ok || got.ID != wantDM.ID {
		t.Errorf("Conversation(%q) = %+v, %v; want the DM found", wantDM.ID, got, ok)
	}

	if got, ok := ws.Conversation("does-not-exist"); ok {
		t.Errorf("Conversation(missing) = %+v, true; want ok=false", got)
	}
}

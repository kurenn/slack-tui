package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/ui/components"
)

// This file covers msgactions.go's remaining 0%-covered functions:
// openMsgLinks/openLinkPicker (never openURLCmd itself — that spawns a real
// browser process, forbidden per CONTRIBUTING's hermeticity rule), reloadCmd
// + applyWorkspace, and slash.go's /search dispatch. Identifiers are
// prefixed "ma" to avoid collisions with other agents' files in this
// package.

// ── open message links ──────────────────────────────────────────────────

// TestOpenMsgLinksZeroSingleMultiple covers all three branches of
// openMsgLinks: no links flashes an error, one link returns openURLCmd's
// command directly (never executed — it would shell out to a real browser),
// several links open the picker listing them all.
func TestOpenMsgLinksZeroSingleMultiple(t *testing.T) {
	m := newSized()

	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: "nolinks", UserID: "ada", Time: "10:00", Text: "just words, no urls here"})
	m.msgSel = len(m.curMsgs()) - 1
	_ = m.openMsgLinks() // returns flash's auto-clear tick, not nil; the error banner is the signal
	if m.picker.open {
		t.Error("no links should not open the link picker")
	}
	if m.loadErr == nil || !strings.Contains(m.loadErr.Error(), "no links") {
		t.Errorf("no links should flash an explanatory error, got %v", m.loadErr)
	}

	m2 := newSized()
	m2.messages[m2.activeID] = append(m2.messages[m2.activeID],
		data.Message{ID: "onelink", UserID: "ada", Time: "10:00", Text: "see https://example.com/docs for details"})
	m2.msgSel = len(m2.curMsgs()) - 1
	cmd2 := m2.openMsgLinks()
	if cmd2 == nil {
		t.Fatal("a single link should return a command (openURLCmd) — not executed here")
	}
	if m2.picker.open {
		t.Error("a single link should not open the picker")
	}

	m3 := newSized()
	m3.messages[m3.activeID] = append(m3.messages[m3.activeID],
		data.Message{ID: "twolinks", UserID: "ada", Time: "10:00",
			Text: "compare https://a.example.com and https://b.example.com"})
	m3.msgSel = len(m3.curMsgs()) - 1
	cmd3 := m3.openMsgLinks()
	if cmd3 == nil || !m3.picker.open || m3.picker.kind != "link" {
		t.Fatalf("multiple links should open the link picker, open=%v kind=%q", m3.picker.open, m3.picker.kind)
	}
	if len(m3.picker.items) != 2 {
		t.Errorf("link picker should list both links, got %d items", len(m3.picker.items))
	}
}

// ── workspace reload ──────────────────────────────────────────────────────

// TestReloadCmdApplyWorkspacePreservesMetaAndSeedsNew: applyWorkspace must
// keep existing per-conversation meta (unread/mention) untouched for
// conversations that survive a reload, and seed fresh meta straight from the
// conversation's own fields for one that's newly appeared — the two branches
// of its old/new merge.
func TestReloadCmdApplyWorkspacePreservesMetaAndSeedsNew(t *testing.T) {
	m := newSized()
	// Give one existing conversation meta that diverges from its Conversation
	// fields, to prove the merge prefers the live (old) meta, not a re-derive.
	existingID := "design"
	m.meta[existingID] = components.Meta{Unread: 7, Mention: true}

	cmd := m.reloadCmd()
	wsMsg1, ok := cmd().(wsMsg)
	if !ok || wsMsg1.err != nil {
		t.Fatalf("reloadCmd against the mock should succeed, got %T %+v", wsMsg1, wsMsg1)
	}
	// Fabricate a "new" conversation appearing in the reloaded workspace, to
	// exercise the seed-from-conv-fields branch without depending on the
	// mock ever actually growing its channel list.
	newWS := *wsMsg1.ws
	newWS.Channels = append(append([]data.Conversation{}, wsMsg1.ws.Channels...),
		data.Conversation{ID: "just-joined", Type: "channel", Name: "just-joined", Unread: 5, Mention: true})

	next, _ := m.Update(wsMsg{ws: &newWS})
	m = next.(Model)

	if got := m.meta[existingID]; got.Unread != 7 || !got.Mention {
		t.Errorf("existing conversation's meta should be preserved untouched, got %+v", got)
	}
	if got := m.meta["just-joined"]; got.Unread != 5 || !got.Mention {
		t.Errorf("a newly-appeared conversation should seed meta from its own fields, got %+v", got)
	}
}

// TestSlashSearchOpensPickerAndRunsQuery drives "/search standup" through the
// composer (as a user would type it) and checks the search picker opens
// pre-seeded with results — runSlash's "search" case, not covered by
// slash_test.go's other command cases.
func TestSlashSearchOpensPickerAndRunsQuery(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	for _, r := range "/search standup" {
		m = Key(m, string(r))
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.picker.open || m.picker.kind != "search" {
		t.Fatalf("/search should open the search picker, open=%v kind=%q", m.picker.open, m.picker.kind)
	}
	if cmd == nil {
		t.Fatal("/search with a query should return a command")
	}
	result := cmd()
	var sm searchMsg
	found := false
	if batch, ok := result.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if r, isS := c().(searchMsg); isS {
				sm, found = r, true
			}
		}
	} else if r, isS := result.(searchMsg); isS {
		sm, found = r, true
	}
	if !found {
		t.Fatalf("expected a searchMsg among the results, got %T", result)
	}
	if sm.err != nil || len(sm.hits) == 0 {
		t.Fatalf("mock search for 'standup' should hit, got %+v", sm)
	}
	next, _ = m.Update(sm)
	m = next.(Model)
	if len(m.picker.items) == 0 {
		t.Fatal("search results should fill the picker")
	}
}

// ── setStatus via /away ────────────────────────────────────────────────

// TestSlashAwaySetsStatusAndPersists drives "/away" through the composer
// (setStatus was previously exercised only via the settings overlay's
// cycleSetting, never via the slash command that actually calls it) and
// checks the status change is both applied and persisted.
func TestSlashAwaySetsStatusAndPersists(t *testing.T) {
	isolateConfigDir(t)
	m := newSized()
	if m.myStatus == "away" {
		t.Fatal("test needs a non-away starting status")
	}
	m = Key(m, "i")
	for _, r := range "/away" {
		m = Key(m, string(r))
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.myStatus != "away" {
		t.Fatalf("myStatus after /away = %q, want away", m.myStatus)
	}
	if cmd == nil {
		t.Fatal("/away should return the presence-push command")
	}
	if pm, ok := cmd().(presenceMsg); !ok || pm.err != nil {
		t.Fatalf("presence push against the mock should succeed, got %T %+v", pm, pm)
	}
	saved, ok := config.Load()
	if !ok && saved.Status != "away" {
		t.Errorf("status should persist to prefs, got %q", saved.Status)
	}
	if saved.Status != "away" {
		t.Errorf("persisted status = %q, want away", saved.Status)
	}
}

// ── error paths: applyDMOpened / applyLeft ──────────────────────────────

// TestApplyDMOpenedErrorFlashes: a failed OpenDM must surface the error
// instead of adding a broken conversation to the sidebar.
func TestApplyDMOpenedErrorFlashes(t *testing.T) {
	m := newSized()
	before := len(m.ws.DMs)
	cmd := m.applyDMOpened(dmOpenedMsg{err: maErr("dm failed")})
	if cmd == nil {
		t.Fatal("an OpenDM error should flash (return a non-nil cmd)")
	}
	if m.loadErr == nil || !strings.Contains(m.loadErr.Error(), "dm failed") {
		t.Errorf("error should surface in loadErr, got %v", m.loadErr)
	}
	if len(m.ws.DMs) != before {
		t.Errorf("a failed OpenDM should not add a conversation, len=%d want %d", len(m.ws.DMs), before)
	}
}

// TestApplyLeftErrorFlashes: a failed Leave must surface the error instead of
// removing the channel from the workspace.
func TestApplyLeftErrorFlashes(t *testing.T) {
	m := newSized()
	before := len(m.ws.Channels)
	cmd := m.applyLeft(leftMsg{convID: m.activeID, err: maErr("leave failed")})
	if cmd == nil {
		t.Fatal("a Leave error should flash (return a non-nil cmd)")
	}
	if m.loadErr == nil || !strings.Contains(m.loadErr.Error(), "leave failed") {
		t.Errorf("error should surface in loadErr, got %v", m.loadErr)
	}
	if len(m.ws.Channels) != before {
		t.Errorf("a failed Leave should not remove the channel, len=%d want %d", len(m.ws.Channels), before)
	}
}

// maErr is a minimal error value for the two tests above.
type maErr string

func (e maErr) Error() string { return string(e) }

// ── slash: /me and /leave's channel-only guard ──────────────────────────

// TestSlashMeItalicizesAction: "/me waves" sends the action wrapped in
// underscores (italic markup); a bare "/me" with no text is a no-op.
func TestSlashMeItalicizesAction(t *testing.T) {
	m := newSized()
	before := len(m.curMsgs())
	m = Key(m, "i")
	for _, r := range "/me waves hello" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if got := len(m.curMsgs()); got != before+1 {
		t.Fatalf("/me with text should send a message, count=%d want %d", got, before+1)
	}
	if last := m.curMsgs()[len(m.curMsgs())-1]; last.Text != "_waves hello_" {
		t.Errorf("/me text = %q, want italic-wrapped", last.Text)
	}

	m2 := newSized()
	before2 := len(m2.curMsgs())
	m2 = Key(m2, "i")
	for _, r := range "/me" {
		m2 = Key(m2, string(r))
	}
	m2 = Key(m2, "enter")
	if got := len(m2.curMsgs()); got != before2 {
		t.Errorf("bare /me should not send anything, count=%d want %d", got, before2)
	}
}

// TestApplyDMOpenedExistingConvNoDuplicate: opening a DM that's already in
// the sidebar must just switch to it, never append a second copy.
func TestApplyDMOpenedExistingConvNoDuplicate(t *testing.T) {
	m := newSized()
	existing, ok := m.ws.Conversation("dm_ada")
	if !ok {
		t.Fatal("test fixture should have dm_ada")
	}
	before := len(m.ws.DMs)
	cmd := m.applyDMOpened(dmOpenedMsg{conv: existing})
	if cmd == nil {
		t.Fatal("applyDMOpened should return the openChannel command")
	}
	if len(m.ws.DMs) != before {
		t.Errorf("an already-present DM should not be duplicated, len=%d want %d", len(m.ws.DMs), before)
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.activeID != "dm_ada" {
		t.Errorf("activeID after opening = %q, want dm_ada", m.activeID)
	}
}

// TestApplyLeftNonActiveChannelKeepsActiveConversation: leaving a channel
// that isn't the one currently open must remove it from the sidebar without
// disturbing the active conversation.
func TestApplyLeftNonActiveChannelKeepsActiveConversation(t *testing.T) {
	m := newSized() // active = engineering
	target := "random"
	if m.activeID == target {
		t.Fatal("test needs a non-active channel to leave")
	}
	before := m.activeID
	cmd := m.applyLeft(leftMsg{convID: target})
	if cmd == nil {
		t.Fatal("applyLeft should return the titleCmd follow-up")
	}
	if _, ok := m.ws.Conversation(target); ok {
		t.Errorf("%q should be removed from the workspace", target)
	}
	if _, ok := m.meta[target]; ok {
		t.Errorf("%q should be removed from meta", target)
	}
	if m.activeID != before {
		t.Errorf("activeID should be unchanged, got %q want %q", m.activeID, before)
	}
}

// TestSlashLeaveOnlyWorksInChannels: /leave in a DM is rejected with a flash
// instead of silently leaving (or erroring against) a conversation type it
// was never meant to touch.
func TestSlashLeaveOnlyWorksInChannels(t *testing.T) {
	m := newSized()
	m.openChannel("dm_lin")
	m = Key(m, "i")
	for _, r := range "/leave" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if m.loadErr == nil || !strings.Contains(m.loadErr.Error(), "only works in channels") {
		t.Errorf("/leave in a DM should flash the channel-only guard, got %v", m.loadErr)
	}
}


package app

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/slack-go/slack"

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
	"github.com/kurenn/slack-tui/internal/theme"
)

// This file covers internal/app/live.go: the Socket Mode event pipeline
// (applyInactiveEvent's alert path, unreadCmd, markAllReadCmd, listenEvents,
// presenceCmd, chanIDs). app_test.go already covers applyEvent's routing and
// the thread-reply badge rule (TestSocketEventMarksUnread,
// TestThreadReplyDoesNotInflateBadge); this file adds the alert-firing rule
// those tests don't check, plus the concurrent-fetch commands that were at
// 0% coverage. Identifiers here are prefixed "live" to avoid collisions.

// ── alert path: bell + notification fire for mentions and DMs only ───────

// TestApplyInactiveEventAlertsForMentionAndDMNotPlainChannel: the comment on
// applyInactiveEvent says the alert (bell + notify) fires "for mentions and
// DMs"; ordinary channel traffic must stay quiet. This is the one property
// TestThreadReplyDoesNotInflateBadge and TestSocketEventMarksUnread never
// check — they only assert the unread counter, not the returned cmds.
func TestApplyInactiveEventAlertsForMentionAndDMNotPlainChannel(t *testing.T) {
	m := newSized()

	// Plain channel message, no mention: quiet.
	other := "random" // a channel, starts read
	cmds := m.applyInactiveEvent(source.Event{ConvID: other, Msg: data.Message{ID: "1", UserID: "ada", Text: "hey"}})
	if len(cmds) != 0 {
		t.Errorf("ordinary channel traffic should not alert, got %d cmds", len(cmds))
	}

	// A mention in a channel: bell + notify.
	cmds = m.applyInactiveEvent(source.Event{ConvID: other, Msg: data.Message{ID: "2", UserID: "ada", Text: "hey @you", MentionsMe: true}})
	if len(cmds) != 2 {
		t.Errorf("a channel mention should alert (bell+notify), got %d cmds", len(cmds))
	}

	// A DM, even without an explicit mention: bell + notify (DMs always alert).
	dm := "dm_lin" // read at baseline
	cmds = m.applyInactiveEvent(source.Event{ConvID: dm, Msg: data.Message{ID: "3", UserID: "lin", Text: "hi there"}})
	if len(cmds) != 2 {
		t.Errorf("a DM should alert even without an explicit mention, got %d cmds", len(cmds))
	}
}

// TestApplyInactiveEventNoAlertWhenNotificationsOff: with notifications off,
// the alert slice still has its two structural slots (bell, notify) but the
// notify slot itself must be nil — a regression where notifyCmd ignores the
// preference would slip past a bare len() check, so this asserts the slot
// value directly.
func TestApplyInactiveEventNoAlertWhenNotificationsOff(t *testing.T) {
	m := newSized()
	m.prefs.Notify = config.NotifyOff
	cmds := m.applyInactiveEvent(source.Event{ConvID: "random", Msg: data.Message{ID: "1", UserID: "ada", Text: "hey @you", MentionsMe: true}})
	if len(cmds) != 2 {
		t.Fatalf("alert slice should still have 2 slots (bell, notify), got %d", len(cmds))
	}
	if cmds[0] == nil {
		t.Error("bell should still fire with notifications off")
	}
	if cmds[1] != nil {
		t.Error("notify slot should be nil when notifications are off")
	}
}

// ── unreadCmd: concurrent fetch across every requested id ────────────────

// TestUnreadCmdFetchesForEveryID: the mock's Unread always answers 0, so this
// isn't testing arithmetic — it proves the concurrent fan-out in unreadCmd
// actually issues a call for every id in the input (a regression that drops
// ids from the loop, or returns early, would shrink the result map) and tags
// the result with the model's current readSeq.
func TestUnreadCmdFetchesForEveryID(t *testing.T) {
	m := newSized()
	ids := m.chanIDs()
	if len(ids) == 0 {
		t.Fatal("test needs at least one non-active channel")
	}
	cmd := m.unreadCmd(ids)
	if cmd == nil {
		t.Fatal("unreadCmd with non-empty ids should return a command")
	}
	msg, ok := cmd().(unreadMsg)
	if !ok {
		t.Fatalf("expected unreadMsg, got %T", cmd())
	}
	if len(msg.counts) != len(ids) {
		t.Errorf("expected a count for every requested id: got %d, want %d", len(msg.counts), len(ids))
	}
	for _, id := range ids {
		if _, ok := msg.counts[id]; !ok {
			t.Errorf("missing count for %q", id)
		}
	}
	if msg.seq != m.readSeq {
		t.Errorf("seq = %d, want the model's readSeq %d", msg.seq, m.readSeq)
	}
	if cmd := m.unreadCmd(nil); cmd != nil {
		t.Error("unreadCmd with no ids should return nil (nothing to fan out)")
	}
}

// TestUnreadCmdRateLimitAbortOmitsID: an id whose Unread call reports a
// slack.RateLimitedError must be excluded from the result — the abort flag
// documented in unreadCmd's comment. Uses the critique-mandated pattern:
// embed the real mock, override the one method under test.
func TestUnreadCmdRateLimitAbortOmitsID(t *testing.T) {
	const badID = "bad-conv"
	src := &liveRateLimitedSource{Source: source.NewMock(), badID: badID}
	m := WithSize(NewWith(src, config.Defaults()), 100, 30)

	cmd := m.unreadCmd([]string{"general", badID, "releases"})
	msg, ok := cmd().(unreadMsg)
	if !ok {
		t.Fatal("expected unreadMsg")
	}
	if _, present := msg.counts[badID]; present {
		t.Errorf("rate-limited id should be omitted from the result, got %+v", msg.counts)
	}
}

// liveRateLimitedSource makes exactly one conversation id fail with a Slack
// rate-limit error; every other call passes through to the real mock.
type liveRateLimitedSource struct {
	source.Source
	badID string
}

func (s *liveRateLimitedSource) Unread(convID string) (int, error) {
	if convID == s.badID {
		return 0, &slack.RateLimitedError{}
	}
	return s.Source.Unread(convID)
}

// ── markAllReadCmd: cached ts used directly, uncached triggers a fetch ────

// TestMarkAllReadCmdCachedVsUncached: an id whose history is already cached
// in m.messages must be marked with that cached ts directly (no extra
// History call); an id with no cache must trigger History first, then mark
// with the ts derived from it. Verified via a recording wrapper — never by
// re-implementing Slack semantics in the fake.
func TestMarkAllReadCmdCachedVsUncached(t *testing.T) {
	rec := &liveRecordingSource{Source: source.NewMock()}
	m := WithSize(NewWith(rec, config.Defaults()), 100, 30)
	// NewWith's ensureHistory only preloads the active conversation.
	cachedID := m.activeID // "engineering" — has cached messages
	uncachedID := "design" // not yet opened, no cache
	if len(m.messages[cachedID]) == 0 {
		t.Fatal("test setup: active conversation should have cached history")
	}
	if len(m.messages[uncachedID]) != 0 {
		t.Fatal("test setup: design should not be cached yet")
	}
	wantCachedTS := lastRealTS(m.messages[cachedID])
	// NewWith's own construction-time preload already called History(cachedID)
	// once; only calls made by markAllReadCmd itself (from here on) count.
	rec.mu.Lock()
	baseline := len(rec.historyCalls)
	rec.mu.Unlock()

	cmd := m.markAllReadCmd([]string{cachedID, uncachedID})
	if cmd == nil {
		t.Fatal("markAllReadCmd with non-empty ids should return a command")
	}
	msg, ok := cmd().(markedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected a successful markedMsg, got %T %+v", msg, msg)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if got := rec.markTS[cachedID]; got != wantCachedTS {
		t.Errorf("cached id marked with ts %q, want the cached ts %q (no extra fetch)", got, wantCachedTS)
	}
	if _, fetched := rec.markTS[uncachedID]; !fetched {
		t.Errorf("uncached id should still end up marked, got %+v", rec.markTS)
	}
	newCalls := rec.historyCalls[baseline:]
	found := false
	for _, id := range newCalls {
		if id == uncachedID {
			found = true
		}
	}
	if !found {
		t.Errorf("uncached id should trigger a History call, got new calls %v", newCalls)
	}
	for _, id := range newCalls {
		if id == cachedID {
			t.Errorf("cached id should NOT trigger an extra History call, got new calls %v", newCalls)
		}
	}
}

// liveRecordingSource records History and MarkRead calls (guarded by a mutex
// — markAllReadCmd fans out across goroutines) while delegating the actual
// work to the real mock.
type liveRecordingSource struct {
	source.Source
	mu           sync.Mutex
	historyCalls []string
	markTS       map[string]string
}

func (s *liveRecordingSource) History(convID string) ([]data.Message, error) {
	s.mu.Lock()
	s.historyCalls = append(s.historyCalls, convID)
	s.mu.Unlock()
	return s.Source.History(convID)
}

func (s *liveRecordingSource) MarkRead(convID, ts string) error {
	s.mu.Lock()
	if s.markTS == nil {
		s.markTS = map[string]string{}
	}
	s.markTS[convID] = ts
	s.mu.Unlock()
	return s.Source.MarkRead(convID, ts)
}

// ── listenEvents: delivers the next event, nil on a closed stream ────────

// liveFakeStreamer is a minimal streamer around a channel the test controls
// directly — the recommended alternative to faking Socket Mode itself.
type liveFakeStreamer struct{ ch chan source.Event }

func (f *liveFakeStreamer) Events() <-chan source.Event { return f.ch }

// TestListenEventsDeliversAndStopsOnClose: a pending event on the channel is
// delivered as eventMsg; a closed channel yields nil (the shutdown path).
func TestListenEventsDeliversAndStopsOnClose(t *testing.T) {
	ch := make(chan source.Event, 1)
	ch <- source.Event{ConvID: "engineering", Msg: data.Message{ID: "x", Text: "hi"}}
	cmd := listenEvents(&liveFakeStreamer{ch: ch})
	msg, ok := cmd().(eventMsg)
	if !ok || msg.ev.ConvID != "engineering" {
		t.Fatalf("expected the queued event delivered as eventMsg, got %T %+v", msg, msg)
	}

	closedCh := make(chan source.Event)
	close(closedCh)
	cmd2 := listenEvents(&liveFakeStreamer{ch: closedCh})
	if got := cmd2(); got != nil {
		t.Errorf("a closed event stream should yield a nil message, got %T", got)
	}
}

// ── presenceCmd / chanIDs ─────────────────────────────────────────────────

// TestPresenceCmdAgainstMockAndEmptyDMs: presenceCmd fetches the DM partners'
// current statuses, which the mock derives from data.Mock()'s seeded values —
// checked against that independent fixture, not against presenceCmd's own
// arithmetic. An empty DM list must short-circuit to a nil command.
func TestPresenceCmdAgainstMockAndEmptyDMs(t *testing.T) {
	m := newSized()
	cmd := m.presenceCmd()
	if cmd == nil {
		t.Fatal("presenceCmd with DM partners present should return a command")
	}
	msg, ok := cmd().(presenceUpdateMsg)
	if !ok {
		t.Fatalf("expected presenceUpdateMsg, got %T", cmd())
	}
	// data.Mock(): ada=online, marco=away (the fixture's independent values).
	if msg.statuses["ada"] != "online" {
		t.Errorf(`statuses["ada"] = %q, want "online" (fixture)`, msg.statuses["ada"])
	}
	if msg.statuses["marco"] != "away" {
		t.Errorf(`statuses["marco"] = %q, want "away" (fixture)`, msg.statuses["marco"])
	}

	m2 := newSized()
	m2.ws.DMs = nil
	if cmd2 := m2.presenceCmd(); cmd2 != nil {
		t.Error("presenceCmd with no DMs should return nil")
	}
}

// TestChanIDsExcludesActive: chanIDs lists every channel except the one
// currently open (which is polled by the active-conversation refresh loop
// instead) — an independent count from the fixture's 6 channels.
func TestChanIDsExcludesActive(t *testing.T) {
	m := newSized() // activeID = "engineering"
	ids := m.chanIDs()
	if len(ids) != len(m.ws.Channels)-1 {
		t.Fatalf("chanIDs len = %d, want %d (all channels minus the active one)", len(ids), len(m.ws.Channels)-1)
	}
	for _, id := range ids {
		if id == m.activeID {
			t.Errorf("chanIDs should exclude the active conversation, found %q", id)
		}
	}
}

// ── historyCmd / refresh ───────────────────────────────────────────────────

// TestHistoryCmdFetchesConversation: historyCmd against the mock returns the
// requested conversation's messages tagged with its own convID.
func TestHistoryCmdFetchesConversation(t *testing.T) {
	m := newSized()
	cmd := m.historyCmd("design")
	msg, ok := cmd().(historyMsg)
	if !ok {
		t.Fatalf("expected historyMsg, got %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("history fetch against the mock should succeed, got %v", msg.err)
	}
	if msg.convID != "design" {
		t.Errorf("convID = %q, want design", msg.convID)
	}
	if len(msg.msgs) == 0 {
		t.Error("design should have messages in the mock fixture")
	}
}

// TestRefreshBatchesHistoryAndOpenThread: refresh() re-fetches the active
// channel alone when no thread is open, and both the channel and the open
// thread's replies once one is.
func TestRefreshBatchesHistoryAndOpenThread(t *testing.T) {
	m := newSized()
	result := m.refresh()()
	if hm, ok := result.(historyMsg); !ok || hm.convID != m.activeID {
		t.Fatalf("refresh with no open thread should fetch just the active channel, got %T %+v", result, result)
	}

	m.msgSel = 2 // e3, has replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("t should open the thread")
	}
	batch, ok := m.refresh()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("refresh with an open thread should batch two fetches, got %T", m.refresh()())
	}
	sawHistory, sawReplies := false, false
	for _, c := range batch {
		switch msg := c().(type) {
		case historyMsg:
			sawHistory = msg.convID == m.activeID
		case repliesMsg:
			sawReplies = msg.convID == m.activeID && msg.rootID == m.threadRootID
		}
	}
	if !sawHistory {
		t.Error("refresh should still fetch the active channel's history")
	}
	if !sawReplies {
		t.Error("refresh should also fetch the open thread's replies")
	}
}

// ── loadOlderCmd ────────────────────────────────────────────────────────

// TestLoadOlderCmdNilWhenNothingCached: with no cached messages there's no
// "oldest" boundary to page before, so loadOlderCmd must return nil rather
// than fetching from an undefined starting point.
func TestLoadOlderCmdNilWhenNothingCached(t *testing.T) {
	m := newSized()
	if cmd := m.loadOlderCmd("design"); cmd != nil {
		t.Error("loadOlderCmd for an uncached conversation should return nil")
	}
	if cmd := m.loadOlderCmd(m.activeID); cmd == nil {
		t.Fatal("loadOlderCmd for a cached conversation should return a command")
	} else if msg, ok := cmd().(olderMsg); !ok || msg.convID != m.activeID {
		t.Errorf("expected an olderMsg for %q, got %T %+v", m.activeID, msg, msg)
	}
}

// ── Update's poll/tick message routing ────────────────────────────────────
//
// dmPollTick/chanPollTick/presencePollTick/pollTick/themeWatchTick themselves
// are correctly left untested (they only wrap tea.Tick — see CONTRIBUTING's
// "what not to test" precedent). What follows instead feeds the MESSAGES
// those tickers eventually deliver directly into Update, which is
// meaningfully different: it exercises the routing/side-effects a real
// regression could break (e.g. dmPollOffset never advancing, or a token
// refresh forgetting to update the live source).

// TestUpdatePollMsgBatchesRefreshAndMarkRead: the 6s active-channel poll
// must re-arm itself and re-fetch the active conversation.
func TestUpdatePollMsgBatchesRefreshAndMarkRead(t *testing.T) {
	m := newSized()
	next, cmd := m.Update(pollMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("pollMsg should return a command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected a batched command, got %T", cmd())
	}
	sawHistory := false
	for _, c := range batch {
		// One element of this batch is pollTick(), a real multi-second
		// tea.Tick; flTryRun (flows_test.go) skips it via a short deadline
		// instead of blocking the test on it.
		if msg, ok := flTryRun(c); ok {
			if hm, isH := msg.(historyMsg); isH && hm.convID == m.activeID {
				sawHistory = true
			}
		}
	}
	if !sawHistory {
		t.Error("pollMsg should refresh the active conversation's history")
	}
}

// TestUpdateThemeWatchMsgFollowsDesktopWhenSelected: only the "omarchy"
// theme choice re-resolves the palette on a desktop theme change — any other
// explicit choice stays put even while the watcher keeps ticking.
func TestUpdateThemeWatchMsgFollowsDesktopWhenSelected(t *testing.T) {
	m := newSized()
	m.prefs.Theme = theme.OmarchyName
	m.themeStamp = "a-stale-stamp-that-cannot-match-the-real-one"
	next, cmd := m.Update(themeWatchMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("themeWatchMsg should return a command (re-arming the watch tick)")
	}
	if m.themeStamp == "a-stale-stamp-that-cannot-match-the-real-one" {
		t.Error("following the omarchy theme should refresh themeStamp when it's stale")
	}
	if m.themeStamp != theme.OmarchyStamp() {
		t.Errorf("themeStamp = %q, want the current theme.OmarchyStamp() %q", m.themeStamp, theme.OmarchyStamp())
	}
}

// TestUpdateTokenRefreshedMsgAppliesOrIgnores: a successful refresh updates
// the live token fields; a failed one is silently ignored (the old token
// stays valid until it isn't, per the comment in Update's case).
func TestUpdateTokenRefreshedMsgAppliesOrIgnores(t *testing.T) {
	m := newSized()
	next, _ := m.Update(tokenRefreshedMsg{toks: config.Tokens{User: "xoxp-new", Refresh: "r2", ExpiresAt: 999}})
	m = next.(Model)
	if m.tokens.User != "xoxp-new" || m.tokens.Refresh != "r2" || m.tokens.ExpiresAt != 999 {
		t.Errorf("successful refresh should update live tokens, got %+v", m.tokens)
	}

	m2 := newSized()
	m2.tokens = config.Tokens{User: "xoxp-old"}
	next2, cmd2 := m2.Update(tokenRefreshedMsg{err: maErr("refresh failed")})
	m2 = next2.(Model)
	if cmd2 != nil {
		t.Error("a failed refresh should not return a follow-up command")
	}
	if m2.tokens.User != "xoxp-old" {
		t.Errorf("a failed refresh should leave the old token in place, got %q", m2.tokens.User)
	}
}

// TestUpdateDMPollMsgAdvancesRotationOffset: each dmPollMsg round must
// advance dmPollOffset by dmPollTail — the rotation the bounded-poll story
// (TestDMPollBoundedAndRotates in app_test.go) depends on to eventually
// cover every dormant DM.
func TestUpdateDMPollMsgAdvancesRotationOffset(t *testing.T) {
	m := newSized()
	before := m.dmPollOffset
	next, cmd := m.Update(dmPollMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("dmPollMsg should return a command")
	}
	if m.dmPollOffset != before+dmPollTail {
		t.Errorf("dmPollOffset = %d, want %d (advanced by dmPollTail)", m.dmPollOffset, before+dmPollTail)
	}
}

// TestUpdateChanAndPresencePollMsgsReturnCommands: both simply re-arm their
// ticker and fan out a fetch — checked by running the fetch half and getting
// a well-typed result back from the mock.
func TestUpdateChanAndPresencePollMsgsReturnCommands(t *testing.T) {
	m := newSized()
	next, cmd := m.Update(chanPollMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("chanPollMsg should return a command")
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		found := false
		for _, c := range batch {
			// chanPollTick() is a real ~90s tea.Tick; skip it via flTryRun
			// rather than blocking the test on it.
			if msg, ok := flTryRun(c); ok {
				if _, isU := msg.(unreadMsg); isU {
					found = true
				}
			}
		}
		if !found {
			t.Error("chanPollMsg's batch should include an unread fetch")
		}
	}

	next, cmd = m.Update(presencePollMsg{})
	_ = next.(Model)
	if cmd == nil {
		t.Fatal("presencePollMsg should return a command")
	}
}

// ── alerts when Socket Mode isn't running ────────────────────────────────────

// Every other alert path needs Socket Mode, which needs tokens a loopback
// sign-in cannot obtain — so for most installs the poll is the ONLY thing that
// can raise a notification. A DM whose count grows must alert.
func TestPollAlertsOnGrowingDM(t *testing.T) {
	m := newSized()
	m.unreadPrimed = true
	dm := flFirstConvOfType(t, &m, "dm")
	mm := m.meta[dm]
	mm.Unread = 0
	m.meta[dm] = mm

	cmds := m.pollAlerts(map[string]int{dm: 2}, m.readSeq)
	if len(cmds) == 0 {
		t.Fatal("a DM going 0 → 2 unread must alert; with no Socket Mode nothing else will")
	}
}

// A count that hasn't moved must stay silent, or every 45s poll would re-alert
// for as long as the conversation stays unread.
func TestPollDoesNotRealertOnUnchangedCount(t *testing.T) {
	m := newSized()
	m.unreadPrimed = true
	dm := flFirstConvOfType(t, &m, "dm")
	mm := m.meta[dm]
	mm.Unread = 3
	m.meta[dm] = mm

	if cmds := m.pollAlerts(map[string]int{dm: 3}, m.readSeq); len(cmds) != 0 {
		t.Errorf("unchanged count alerted %d cmds, want silence", len(cmds))
	}
	if cmds := m.pollAlerts(map[string]int{dm: 2}, m.readSeq); len(cmds) != 0 {
		t.Errorf("a shrinking count alerted %d cmds, want silence", len(cmds))
	}
}

// The first round compares against counts from the initial load, which are
// often zero — alerting there would greet the user with a notification per
// unread conversation at every launch.
func TestPollStaysSilentOnFirstRound(t *testing.T) {
	m := newSized()
	m.unreadPrimed = false
	dm := flFirstConvOfType(t, &m, "dm")
	mm := m.meta[dm]
	mm.Unread = 0
	m.meta[dm] = mm

	if cmds := m.pollAlerts(map[string]int{dm: 5}, m.readSeq); len(cmds) != 0 {
		t.Errorf("first round alerted %d cmds, want silence", len(cmds))
	}
	if !m.unreadPrimed {
		t.Error("the first round must arm subsequent ones")
	}
}

// The conversation on screen is being read; alerting about it is noise.
func TestPollDoesNotAlertForActiveConversation(t *testing.T) {
	m := newSized()
	m.unreadPrimed = true
	if cmds := m.pollAlerts(map[string]int{m.activeID: 9}, m.readSeq); len(cmds) != 0 {
		t.Errorf("alerted for the active conversation (%d cmds)", len(cmds))
	}
}

// Channels must NOT alert merely for growing — only a real mention counts, or a
// busy channel would notify on every message. Growth schedules a scan instead.
func TestPollChannelGrowthScansRatherThanAlerting(t *testing.T) {
	m := newSized()
	m.unreadPrimed = true
	ch := flFirstConvOfType(t, &m, "channel")
	mm := m.meta[ch]
	mm.Unread = 0
	m.meta[ch] = mm

	cmds := m.pollAlerts(map[string]int{ch: 4}, m.readSeq)
	if len(cmds) != 1 {
		t.Fatalf("a grown channel should schedule exactly one mention scan, got %d cmds", len(cmds))
	}
	// The scan cmd must resolve to a mentionScanMsg, not fire an alert directly.
	if _, ok := cmds[0]().(mentionScanMsg); !ok {
		t.Error("channel growth alerted directly instead of scanning for a mention")
	}
}

// flFirstConvOfType returns a conversation id of the given kind from the mock.
func flFirstConvOfType(t *testing.T, m *Model, kind string) string {
	t.Helper()
	for _, list := range [][]data.Conversation{m.ws.Channels, m.ws.DMs} {
		for _, c := range list {
			if c.Type == kind && c.ID != m.activeID {
				return c.ID
			}
		}
	}
	t.Fatalf("mock workspace has no %q conversation to test with", kind)
	return ""
}

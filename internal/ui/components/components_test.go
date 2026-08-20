package components

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/theme"
)

// smPalette resolves the built-in charcoal theme — every test below anchors
// its color assertions to this one concrete, resolved palette (via smFgOpen/
// smBgOpen below) rather than to a hardcoded escape sequence, so the
// assertions survive a theme-table edit and still fail on a real regression
// in which field a component reaches for.
func smPalette() theme.Palette { return theme.Resolve("charcoal", "auto") }

// smFgOpen/smBgOpen return the literal ANSI escape lipgloss emits to open a
// foreground/background color under the truecolor profile testenv.Pin
// forces. They're built by calling lipgloss exactly as production code does,
// so a test can assert "this specific palette color reached the output"
// without hand-computing termenv's lossy float->uint8 RGB rounding itself.
func smFgOpen(c lipgloss.Color) string {
	s := lipgloss.NewStyle().Foreground(c).Render("\x00")
	return s[:strings.IndexByte(s, '\x00')]
}

func smBgOpen(c lipgloss.Color) string {
	s := lipgloss.NewStyle().Background(c).Render("\x00")
	return s[:strings.IndexByte(s, '\x00')]
}

// smFgFragment returns just the color parameters of the foreground escape
// (e.g. "38;2;121;131;143", no leading "\x1b[" or trailing "m"), so it still
// matches when lipgloss folds a foreground *and* background into one combined
// SGR sequence (as TitleBar's `bg.Foreground(...)` does) — smFgOpen's full
// escape wouldn't appear verbatim in that case even though the same color is
// in effect.
func smFgFragment(c lipgloss.Color) string {
	open := smFgOpen(c)
	return strings.TrimSuffix(strings.TrimPrefix(open, "\x1b["), "m")
}

// smThemeBgOpen returns the background-set escape exactly as
// internal/theme.FillBg builds it (via colorful's RGB255, which rounds with
// +0.5) — NOT how lipgloss/termenv's own Background() renders it (which
// truncates without rounding, per termenv's RGBColor.Sequence). The two can
// differ by one part in 255 for the same hex, so components that paint their
// highlight via theme.FillBg (sideRow, messageLines) must be checked against
// this, while components that call lipgloss's Background() directly
// (paletteRow, StatusBar, TitleBar) are checked against smBgOpen instead.
func smThemeBgOpen(c lipgloss.Color) string {
	col, err := colorful.Hex(string(c))
	if err != nil {
		return ""
	}
	r, g, b := col.RGB255()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// smWorkspace builds a small, fully-controlled workspace fixture (not
// data.Mock()) so every assertion below traces to a value this file wrote,
// not a literal from the shared sample data that could change independently.
func smWorkspace() *data.Workspace {
	return &data.Workspace{
		MeID: "me",
		Users: map[string]data.User{
			"me":   {ID: "me", Name: "you", Color: "fg", Status: "online"},
			"ada":  {ID: "ada", Name: "ada", Color: "purple", Status: "online"},
			"lin":  {ID: "lin", Name: "lin", Color: "green", Status: "away"},
			"kip":  {ID: "kip", Name: "kip", Color: "blue", Status: "dnd"},
			"remy": {ID: "remy", Name: "remy", Color: "cyan", Status: "offline"},
		},
		Channels: []data.Conversation{
			{ID: "c1", Type: "channel", Name: "general", Unread: 0},
			{ID: "c2", Type: "channel", Name: "eng", Unread: 3},
		},
		DMs: []data.Conversation{
			{ID: "d1", Type: "dm", Name: "ada", UserID: "ada", Unread: 0},
			{ID: "d2", Type: "dm", Name: "lin", UserID: "lin", Unread: 2, Mention: true},
		},
	}
}

// ---- windowStart ----

func TestWindowStart(t *testing.T) {
	tests := []struct {
		name                  string
		index, total, maxRows int
		want                  int
	}{
		{"total fits entirely, no scroll needed", 2, 5, 10, 0},
		{"index at the very top", 0, 20, 5, 0},
		{"index in the middle centers the window", 10, 20, 5, 8}, // 10 - 5/2 = 8
		{"index at the very bottom pins the window to the end", 19, 20, 5, 15},
		{"index one past the top edge still clamps to 0", 1, 20, 5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowStart(tc.index, tc.total, tc.maxRows); got != tc.want {
				t.Errorf("windowStart(%d, %d, %d) = %d, want %d", tc.index, tc.total, tc.maxRows, got, tc.want)
			}
		})
	}
}

// ---- Palette / paletteRow ----

func smPaletteItems(n int) []PaletteItem {
	items := make([]PaletteItem, n)
	for i := range items {
		items[i] = PaletteItem{Icon: "#", Label: "item " + string(rune('0'+i)), Hint: "go", Kind: "channel"}
	}
	return items
}

// With more items than fit, the window must scroll to keep the selected row
// visible and the far-off first item must scroll out of view — otherwise a
// long list would either hide the selection or never actually scroll.
func TestPaletteWindowsAroundSelection(t *testing.T) {
	p := smPalette()
	items := smPaletteItems(10)
	out := ansi.Strip(Palette(p, "", items, 8, 40, 4))

	if !strings.Contains(out, "item "+string(rune('0'+8))) {
		t.Errorf("selected item 8 should be visible, got:\n%s", out)
	}
	if strings.Contains(out, "item 0") {
		t.Errorf("item 0 should have scrolled out of the window, got:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "┌") || !strings.Contains(lines[len(lines)-1], "└") {
		t.Errorf("box corners missing on first/last line:\nfirst=%q\nlast=%q", lines[0], lines[len(lines)-1])
	}
}

func TestPaletteEmptyShowsNoMatches(t *testing.T) {
	out := ansi.Strip(Palette(smPalette(), "", nil, 0, 40, 5))
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected 'no matches' for an empty item list, got:\n%s", out)
	}
}

// Every line of the box must respect the requested width, even when a label
// is far longer than the box — a truncation regression would blow out the
// frame and corrupt everything drawn after it.
func TestPaletteTruncatesLongLabels(t *testing.T) {
	items := []PaletteItem{{Icon: "#", Label: strings.Repeat("very-long-channel-name-", 5), Hint: "go", Kind: "channel"}}
	out := Palette(smPalette(), "", items, 0, 30, 3)
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 30 {
			t.Errorf("line %d width = %d, want <= 30: %q", i, w, ansi.Strip(line))
		}
	}
}

// The selected row must be painted with the palette's selection background
// and the unselected row with the panel background — not the same color —
// or the cursor would be invisible in the command palette.
func TestPaletteRowSelectedVsUnselectedBackground(t *testing.T) {
	p := smPalette()
	item := PaletteItem{Icon: "#", Label: "general", Hint: "channel", Kind: "channel"}

	selected := paletteRow(p, item, true, 30)
	unselected := paletteRow(p, item, false, 30)

	if selected == unselected {
		t.Fatal("selected and unselected rows rendered identically")
	}
	if !strings.Contains(selected, smBgOpen(p.SelBg)) {
		t.Errorf("selected row should carry the SelBg background, got:\n%q", selected)
	}
	if strings.Contains(unselected, smBgOpen(p.SelBg)) {
		t.Errorf("unselected row should NOT carry SelBg, got:\n%q", unselected)
	}
	if !strings.Contains(unselected, smBgOpen(p.Panel)) {
		t.Errorf("unselected row should carry the Panel background, got:\n%q", unselected)
	}
}

// ---- Composer ----

func TestComposerHintTogglesWithInsertMode(t *testing.T) {
	normal := ansi.Strip(Composer(smPalette(), "»", "", false, 40))
	if !strings.Contains(normal, "i to write") {
		t.Errorf("normal mode should show the 'i to write' hint, got:\n%s", normal)
	}
	insert := ansi.Strip(Composer(smPalette(), "»", "", true, 40))
	if !strings.Contains(insert, "↵ send") {
		t.Errorf("insert mode should show the send hint, got:\n%s", insert)
	}
	if strings.Contains(insert, "i to write") {
		t.Errorf("insert mode should not still show the normal-mode hint, got:\n%s", insert)
	}
}

// A multi-line textarea input must produce one continuation row per input
// line, each still bounded by the box borders.
func TestComposerMultiLineContinuation(t *testing.T) {
	out := Composer(smPalette(), "»", "line one\nline two\nline three", true, 40)
	lines := strings.Split(out, "\n")
	// top border + 3 input rows + bottom border
	if len(lines) != 5 {
		t.Fatalf("expected 5 rendered rows (border+3+border), got %d:\n%s", len(lines), out)
	}
	stripped := ansi.Strip(out)
	for _, want := range []string{"line one", "line two", "line three"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("missing continuation line %q in:\n%s", want, stripped)
		}
	}
}

// At a width too narrow for both the input and the hint, the hint must be
// dropped first so the input text — what the user is actively typing —
// never gets clipped instead.
func TestComposerNarrowWidthDropsHintBeforeTruncatingInput(t *testing.T) {
	input := "hello world"
	// innerW = width-2; prompt+input take up most of a narrow box.
	out := ansi.Strip(Composer(smPalette(), "»", input, false, len(input)+8))
	if strings.Contains(out, "i to write") {
		t.Errorf("hint should have been dropped at this width, got:\n%s", out)
	}
	if !strings.Contains(out, input) {
		t.Errorf("input text should remain intact, got:\n%s", out)
	}
}

// ---- Sidebar: BuildSideItems / SelectableIndexes ----

func TestBuildSideItemsHiddenRemovesRowsNotHeaders(t *testing.T) {
	ws := smWorkspace()
	hidden := map[string]bool{"c2": true, "d1": true}
	items := BuildSideItems(ws, nil, hidden)

	var headers, rows int
	rowIDs := map[string]bool{}
	for _, it := range items {
		if it.Header {
			headers++
			continue
		}
		rows++
		rowIDs[it.Conv.ID] = true
	}
	if headers != 2 {
		t.Errorf("expected both section headers to remain, got %d", headers)
	}
	if rowIDs["c2"] || rowIDs["d1"] {
		t.Errorf("hidden conversations should be omitted, got rows: %v", rowIDs)
	}
	if !rowIDs["c1"] || !rowIDs["d2"] {
		t.Errorf("non-hidden conversations should remain, got rows: %v", rowIDs)
	}
	if rows != 2 {
		t.Errorf("expected 2 visible rows (4 total - 2 hidden), got %d", rows)
	}
}

func TestBuildSideItemsAppliesLiveMeta(t *testing.T) {
	ws := smWorkspace()
	meta := map[string]Meta{"c1": {Unread: 7, Mention: true}}
	items := BuildSideItems(ws, meta, nil)

	found := false
	for _, it := range items {
		if it.Header {
			continue
		}
		if it.Conv.ID == "c1" {
			found = true
			if it.Conv.Unread != 7 || !it.Conv.Mention {
				t.Errorf("c1 meta not applied: unread=%d mention=%v", it.Conv.Unread, it.Conv.Mention)
			}
		} else if it.Conv.ID == "c2" && (it.Conv.Unread != 3) {
			t.Errorf("c2 (no meta entry) should keep its own Unread=3, got %d", it.Conv.Unread)
		}
	}
	if !found {
		t.Fatal("c1 not found in built items")
	}
}

func TestSelectableIndexesSkipsExactlyTheHeaders(t *testing.T) {
	items := BuildSideItems(smWorkspace(), nil, nil)
	idx := SelectableIndexes(items)
	if len(idx) != 4 {
		t.Fatalf("expected 4 selectable rows (2 channels + 2 dms), got %d: %v", len(idx), idx)
	}
	for _, i := range idx {
		if items[i].Header {
			t.Errorf("SelectableIndexes returned a header index %d", i)
		}
	}
}

// ---- Sidebar: SidebarBody / sideRow ----

// An unread conversation must show its count badge; the badge must be
// distinct in kind (an @-mention badge, not a plain count) when the
// conversation has a pending mention.
func TestSideRowUnreadBadgeAndMentionBadgeDiffer(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()

	unread := ws.Channels[1] // c2, Unread: 3, no mention
	mention := ws.DMs[1]     // d2, Unread: 2, Mention: true

	unreadOut := ansi.Strip(sideRow(p, ws, unread, false, false, 40))
	mentionOut := ansi.Strip(sideRow(p, ws, mention, false, false, 40))

	if !strings.Contains(unreadOut, "3") {
		t.Errorf("plain-unread row should show its count, got: %q", unreadOut)
	}
	if strings.Contains(unreadOut, "@3") {
		t.Errorf("plain-unread row should not use the @ mention badge, got: %q", unreadOut)
	}
	if !strings.Contains(mentionOut, "@2") {
		t.Errorf("mention row should show an @-prefixed badge, got: %q", mentionOut)
	}
}

// The cursor row gets an accent bar in the left margin; an active-but-not-
// cursor row does not, even though both share the highlighted background —
// this is the only visual cue distinguishing "here" from "focused pane".
func TestSideRowCursorMarginDistinctFromPlainActive(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	c := ws.Channels[0] // c1: no unread, no mention

	cursorRow := sideRow(p, ws, c, true, false, 40) // cursor, not active
	activeRow := sideRow(p, ws, c, false, true, 40) // active, not cursor
	plainRow := sideRow(p, ws, c, false, false, 40) // neither

	accentBar := smFgOpen(p.Accent) + "▌"
	if !strings.Contains(cursorRow, accentBar) {
		t.Errorf("cursor row should carry the accent bar %q, got: %q", accentBar, cursorRow)
	}
	if strings.Contains(activeRow, accentBar) {
		t.Errorf("a plain active (non-cursor) row should not carry the cursor's accent bar, got: %q", activeRow)
	}
	// Both cursor and active rows highlight the background (painted via
	// theme.FillBg, hence smThemeBgOpen rather than smBgOpen); a fully plain
	// row does not.
	if !strings.Contains(cursorRow, smThemeBgOpen(p.SelBg)) || !strings.Contains(activeRow, smThemeBgOpen(p.SelBg)) {
		t.Error("cursor and active rows should both paint the selection background")
	}
	if strings.Contains(plainRow, smThemeBgOpen(p.SelBg)) {
		t.Errorf("a row that is neither active nor cursor should not paint the selection background, got: %q", plainRow)
	}
}

func TestSidebarBodyTracksCursorLine(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	items := BuildSideItems(ws, nil, nil)
	sel := SelectableIndexes(items)
	// Select the second selectable row (index into the flat item list).
	target := sel[1]

	lines, cursorLine := SidebarBody(p, ws, items, "", target, true, 30)
	if cursorLine <= 0 || cursorLine >= len(lines) {
		t.Fatalf("cursorLine %d out of range for %d lines", cursorLine, len(lines))
	}
	// The reported cursor line must actually be the row for the selected conversation.
	wantLabel := items[target].Conv.Name
	if !strings.Contains(ansi.Strip(lines[cursorLine]), wantLabel) {
		t.Errorf("cursorLine %d = %q, expected it to contain %q", cursorLine, ansi.Strip(lines[cursorLine]), wantLabel)
	}
}

// ---- StatusBar ----

func TestStatusBarModeAndHints(t *testing.T) {
	p := smPalette()
	hints := []Hint{H("dd", "delete"), H("y", "yank")}

	insert := ansi.Strip(StatusBar(p, true, "#general", "you", hints, true, 100))
	if !strings.Contains(insert, "INSERT") {
		t.Errorf("insert mode should show INSERT, got: %q", insert)
	}
	if !strings.Contains(insert, "dd") || !strings.Contains(insert, "delete") {
		t.Errorf("hints should be shown when showHints=true, got: %q", insert)
	}

	normal := ansi.Strip(StatusBar(p, false, "#general", "you", hints, true, 100))
	if !strings.Contains(normal, "NORMAL") || strings.Contains(normal, "INSERT") {
		t.Errorf("normal mode should show NORMAL not INSERT, got: %q", normal)
	}

	hidden := ansi.Strip(StatusBar(p, false, "#general", "you", hints, false, 100))
	if strings.Contains(hidden, "dd") {
		t.Errorf("hints should be hidden when showHints=false, got: %q", hidden)
	}
}

func TestStatusBarRespectsWidth(t *testing.T) {
	out := StatusBar(smPalette(), false, "#general", "you", []Hint{H("q", "quit")}, true, 80)
	if w := lipgloss.Width(out); w != 80 {
		t.Errorf("StatusBar width = %d, want 80", w)
	}
}

// ---- TitleBar ----

func TestTitleBarShowsWorkspaceHandleAndPaneCount(t *testing.T) {
	p := smPalette()
	out := ansi.Strip(TitleBar(p, "acme-labs", "ada", "online", 2, 60))
	for _, want := range []string{"acme-labs", "@ada", "2 panes"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in title bar: %q", want, out)
		}
	}
}

func TestTitleBarPaneCountChangesSegment(t *testing.T) {
	one := ansi.Strip(TitleBar(smPalette(), "acme", "ada", "online", 1, 60))
	three := ansi.Strip(TitleBar(smPalette(), "acme", "ada", "online", 3, 60))
	if !strings.Contains(one, "1 panes") || !strings.Contains(three, "3 panes") {
		t.Errorf("pane count segment did not track panes: one=%q three=%q", one, three)
	}
}

// The presence dot's color must reach the title bar — a status regression
// (e.g. dnd rendering as green) would otherwise be invisible to any test
// that only checks for the glyph.
func TestTitleBarPresenceColorReachesOutput(t *testing.T) {
	p := smPalette()
	out := TitleBar(p, "acme", "ada", "dnd", 1, 60)
	if !strings.Contains(out, smFgFragment(p.Red)) {
		t.Errorf("dnd status should render with the Red presence color, got: %q", out)
	}
}

// ---- PresenceColor / PresenceDot ----

func TestPresenceColorMapping(t *testing.T) {
	p := smPalette()
	tests := map[string]lipgloss.Color{
		"online":  p.Green,
		"away":    p.Yellow,
		"dnd":     p.Red,
		"offline": p.Dim2,
		"":        p.Dim2,
		"bogus":   p.Dim2, // unknown status must fall back, not zero-value
	}
	for status, want := range tests {
		if got := PresenceColor(p, status); got != want {
			t.Errorf("PresenceColor(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestPresenceDotGlyphAndColor(t *testing.T) {
	p := smPalette()
	for _, status := range []string{"online", "away", "dnd"} {
		dot := PresenceDot(p, status)
		if !strings.Contains(ansi.Strip(dot), "●") {
			t.Errorf("status %q should render a filled dot, got %q", status, ansi.Strip(dot))
		}
	}
	for _, status := range []string{"offline", ""} {
		dot := PresenceDot(p, status)
		if !strings.Contains(ansi.Strip(dot), "○") {
			t.Errorf("status %q should render a hollow ring, got %q", status, ansi.Strip(dot))
		}
	}
	// The glyph must carry the matching presence color, tying PresenceDot to
	// PresenceColor rather than hardcoding some other hue.
	online := PresenceDot(p, "online")
	if !strings.Contains(online, smFgOpen(p.Green)) {
		t.Errorf("online dot should use the Green presence color, got %q", online)
	}
}

// ---- Wrap ----

func TestWrapBreaksOnWordsAndHardBreaksOverflow(t *testing.T) {
	out := Wrap("the quick brown fox", 10)
	for _, ln := range out {
		if lipgloss.Width(ln) > 10 {
			t.Errorf("wrapped line exceeds width 10: %q", ln)
		}
	}
	joined := strings.Join(out, " ")
	if !strings.Contains(joined, "quick") || !strings.Contains(joined, "brown") {
		t.Errorf("wrapping should not drop words, got: %v", out)
	}

	// A single "word" with no spaces at all, longer than width, must still be
	// hard-broken rather than overflow the line.
	long := Wrap(strings.Repeat("x", 25), 10)
	for _, ln := range long {
		if lipgloss.Width(ln) > 10 {
			t.Errorf("hard-break should cap every line at width 10, got: %q", ln)
		}
	}
	if strings.Join(long, "") != strings.Repeat("x", 25) {
		t.Errorf("hard-break should not lose or duplicate characters, got: %v", long)
	}
}

// ---- ThreadScroll ----

func TestThreadScrollRootPlusRepliesWithSeparator(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	root := data.Message{
		ID: "m1", UserID: "ada", Time: "09:00", Text: "root question",
		Replies: []data.Reply{
			{ID: "r1", UserID: "lin", Time: "09:01", Text: "first reply"},
			{ID: "r2", UserID: "kip", Time: "09:02", Text: "second reply"},
		},
	}
	lines, starts := ThreadScroll(p, ws, root, 0, true, 50)
	joined := ansi.Strip(strings.Join(lines, "\n"))

	for _, want := range []string{"root question", "first reply", "second reply", "2 replies"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in thread scroll output:\n%s", want, joined)
		}
	}
	if len(starts) != len(root.Replies)+1 {
		t.Errorf("starts should have one entry per reply plus a sentinel, got %d, want %d", len(starts), len(root.Replies)+1)
	}
	// Each reply's start index must actually point at a line containing that reply's text.
	if !strings.Contains(ansi.Strip(lines[starts[0]]), "lin") && !strings.Contains(ansi.Strip(strings.Join(lines[starts[0]:starts[1]], "\n")), "first reply") {
		t.Errorf("starts[0] does not point at the first reply's lines")
	}
}

// ---- messages.go: messageLines details (grouping, attachments, reactions, replies, highlight) ----

func TestMessageLinesGroupsConsecutiveAuthor(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	msgs := []data.Message{
		{ID: "1", UserID: "ada", Time: "09:00", Text: "first"},
		{ID: "2", UserID: "ada", Time: "09:01", Text: "second, same author"},
	}
	lines, _ := MessagesBody(p, ws, msgs, -1, false, theme.Comfortable, 60, "")
	joined := ansi.Strip(strings.Join(lines, "\n"))
	// The author name header should appear exactly once — the second message
	// in the group omits the repeated name/time header.
	if n := strings.Count(joined, "ada"); n != 1 {
		t.Errorf("author name %q should appear exactly once for a grouped pair, appeared %d times in:\n%s", "ada", n, joined)
	}
}

func TestMessageLinesExtraAttachmentsAreDimmed(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	msg := data.Message{ID: "1", UserID: "ada", Time: "09:00", Text: "see attached", Extra: []string{"[file: report.pdf]"}}
	lines, _ := MessagesBody(p, ws, []data.Message{msg}, -1, false, theme.Comfortable, 60, "")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(ansi.Strip(joined), "report.pdf") {
		t.Fatalf("attachment annotation missing from output:\n%s", ansi.Strip(joined))
	}
	// Find the line with the attachment and assert it carries the Dim color.
	var attLine string
	for _, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "report.pdf") {
			attLine = ln
		}
	}
	if !strings.Contains(attLine, smFgOpen(p.Dim)) {
		t.Errorf("attachment line should be dimmed (Dim color), got: %q", attLine)
	}
}

func TestMessageLinesReactionsShowEmojiAndCount(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	msg := data.Message{ID: "1", UserID: "ada", Time: "09:00", Text: "nice", Reactions: []data.Reaction{{Emoji: "🔥", Count: 5}}}
	lines, _ := MessagesBody(p, ws, []data.Message{msg}, -1, false, theme.Comfortable, 60, "")
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "🔥") || !strings.Contains(joined, "5") {
		t.Errorf("expected the reaction emoji and count, got:\n%s", joined)
	}
}

// The reply affordance must preview the LATEST reply (not the first), with
// its author resolved from the workspace's user map.
func TestMessageLinesReplyAffordancePreviewsLatestReply(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	msg := data.Message{
		ID: "1", UserID: "ada", Time: "09:00", Text: "question", ReplyCount: 2,
		Replies: []data.Reply{
			{ID: "r1", UserID: "lin", Time: "09:01", Text: "first answer"},
			{ID: "r2", UserID: "kip", Time: "09:02", Text: "latest answer"},
		},
	}
	lines, _ := MessagesBody(p, ws, []data.Message{msg}, -1, false, theme.Comfortable, 60, "")
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "2 replies") {
		t.Errorf("expected reply count pluralized as 'replies', got:\n%s", joined)
	}
	if !strings.Contains(joined, "kip") || !strings.Contains(joined, "latest answer") {
		t.Errorf("expected the latest reply's author (kip) and text previewed, got:\n%s", joined)
	}
	if strings.Contains(joined, "first answer") {
		t.Errorf("should preview only the latest reply, not the first, got:\n%s", joined)
	}
}

// A selected message gets an accent bar + SelBg background; a mentioned (but
// not selected) message gets an orange bar + MentionBg — and the two must
// not be confusable, or a mention would look like your cursor position.
func TestMessageLinesSelectedVsMentionHighlight(t *testing.T) {
	p := smPalette()
	ws := smWorkspace()
	msgs := []data.Message{{ID: "1", UserID: "ada", Time: "09:00", Text: "hi", MentionsMe: true}}

	selectedLines, _ := MessagesBody(p, ws, msgs, 0, true, theme.Comfortable, 60, "")
	mentionLines, _ := MessagesBody(p, ws, msgs, -1, false, theme.Comfortable, 60, "")

	selected := strings.Join(selectedLines, "\n")
	mention := strings.Join(mentionLines, "\n")

	if !strings.Contains(selected, smFgOpen(p.Accent)+"▌") {
		t.Errorf("selected message should carry the accent bar, got:\n%q", selected)
	}
	if !strings.Contains(selected, smThemeBgOpen(p.SelBg)) {
		t.Errorf("selected message should carry SelBg, got:\n%q", selected)
	}
	if !strings.Contains(mention, smFgOpen(p.Orange)+"▌") {
		t.Errorf("mentioned message should carry the orange bar, got:\n%q", mention)
	}
	if !strings.Contains(mention, smThemeBgOpen(p.MentionBg)) {
		t.Errorf("mentioned message should carry MentionBg, got:\n%q", mention)
	}
	if strings.Contains(mention, smThemeBgOpen(p.SelBg)) {
		t.Errorf("an unselected mention should not carry the selection background, got:\n%q", mention)
	}
}

// ---- replyWho / replyPreview / plural ----

func TestReplyWhoDedupsAndCapsAtThree(t *testing.T) {
	ws := smWorkspace()
	replies := []data.Reply{
		{UserID: "ada"}, {UserID: "ada"}, // duplicate — must appear once
		{UserID: "lin"}, {UserID: "kip"}, {UserID: "remy"}, // 4th unique — must be dropped
	}
	got := replyWho(ws, replies)
	want := "ada, lin, kip"
	if got != want {
		t.Errorf("replyWho = %q, want %q", got, want)
	}
}

func TestReplyPreviewFlattensAndTruncates(t *testing.T) {
	if got := replyPreview("line one\nline two", 100); got != "line one line two" {
		t.Errorf("newlines should collapse to spaces, got %q", got)
	}
	long := strings.Repeat("a", 30)
	// w=15 is above the 12-rune floor, so it drives the truncation directly.
	got := replyPreview(long, 15)
	if r := []rune(got); len(r) != 15 || r[14] != '…' {
		t.Errorf("replyPreview(30 chars, w=15) = %q, want 15 runes ending in an ellipsis", got)
	}
	// w below the 12-rune floor must still clamp to 12, not truncate harder —
	// a preview shorter than 12 runes would be unreadable.
	for _, w := range []int{10, 3, 0} {
		clamped := replyPreview(long, w)
		if r := []rune(clamped); len(r) != 12 || r[11] != '…' {
			t.Errorf("replyPreview(30 chars, w=%d) = %q, want 12 runes ending in an ellipsis (the floor)", w, clamped)
		}
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "reply", "replies") != "reply" {
		t.Error(`plural(1, ...) should be "reply"`)
	}
	if plural(2, "reply", "replies") != "replies" {
		t.Error(`plural(2, ...) should be "replies"`)
	}
	if plural(0, "reply", "replies") != "replies" {
		t.Error(`plural(0, ...) should be "replies" (zero is plural)`)
	}
}

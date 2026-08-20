package onboarding

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/auth"
	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/theme"
)

// This file drives the onboarding flow end-to-end through real key/message
// sequences (never by poking model fields directly for the state under test),
// and renders the phases whose View code was previously never exercised —
// view.go, wizard_view.go, trainer_view.go and typewriter.go account for most
// of the package's uncovered statements. Every assertion below is a semantic
// fact (a specific label, a specific marker's position, a specific hex) that a
// plausible regression would flip — never a full-frame snapshot.

// ── full flow: boot → auth → app-setup (bad id, then valid) → identity →
//    wizard (all steps + keyboard trainer) → launch → finish persists prefs ──

// TestFullFlowThroughWizardToLaunchPersistsPrefs walks the entire onboarding
// flow with real keystrokes for every phase it can complete headlessly. The
// one leg it cannot run for real is the browser OAuth exchange (network); it
// stops at the point slack-tui hands off to the browser (oauthRunning=true,
// the client ID persisted) and then continues the flow the way the browser
// callback would — by delivering oauthDoneMsg, exactly as Update's real
// oauthCmd does on success. Nothing here executes oauthCmd itself.
func TestFullFlowThroughWizardToLaunchPersistsPrefs(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("SLACK_CLIENT_ID", "")
	t.Setenv("SLACK_CLIENT_SECRET", "")

	m := sized()
	m = Key(m, " ") // fast-forward the boot typewriter
	if !m.boot.done {
		t.Fatal("a key during boot should fast-forward it")
	}
	m = update(m, bootAdvanceMsg{})
	if m.phase != phaseAuth {
		t.Fatalf("phase after boot = %q, want auth", m.phase)
	}

	// Sign in with Slack, with no app configured yet → app-setup screen.
	m = Key(m, "1")
	if m.phase != phaseAppSetup {
		t.Fatalf("phase after choosing slack = %q, want appsetup", m.phase)
	}

	// A bad client ID (this is actually the App ID) must be rejected in place.
	for _, k := range strings.Split("A,0,B,8,K,U,L,B,K,8,W,enter", ",") {
		m = Key(m, k)
	}
	if m.phase != phaseAppSetup {
		t.Fatalf("bad client id should not advance, phase = %q", m.phase)
	}
	if got := config.LoadOAuthCreds(); got.ClientID != "" {
		t.Fatalf("a rejected client id must not be saved, got %q", got.ClientID)
	}

	// Clear the bad input, then a real-shaped client ID: 1234567890.9876543210
	m.clientID.SetValue("")
	for _, r := range "1234567890.9876543210" {
		m = Key(m, string(r))
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next
	if m.phase != phaseOAuth || !m.oauthRunning {
		t.Fatalf("valid client id should start oauth, phase=%q running=%v", m.phase, m.oauthRunning)
	}
	if cmd == nil {
		t.Fatal("starting oauth should return a command (tick + oauthCmd)")
	}
	if got := config.LoadOAuthCreds(); got.ClientID != "1234567890.9876543210" {
		t.Fatalf("valid client id should be persisted, got %q", got.ClientID)
	}
	// Deliberately never call cmd() — it batches oauthCmd, which performs a
	// real network login. Continue the flow the way the browser callback
	// would: deliver the outcome message directly.
	next, _ = m.Update(oauthDoneMsg{team: auth.Team{ID: "T999", Name: "Acme Corp"}})
	m = next
	if m.phase != phaseIdentity {
		t.Fatalf("successful oauth should land on identity, phase = %q", m.phase)
	}
	if ws, active, err := config.LoadWorkspaces(); err != nil || active != "acme-corp" {
		t.Fatalf("oauth success should persist+activate the slugified workspace name, active=%q err=%v ws=%+v", active, err, ws)
	}

	// Identity: empty handle is blocked, then a real handle advances.
	m = Key(m, "enter")
	if m.phase != phaseIdentity {
		t.Fatal("empty handle should not advance past identity")
	}
	for _, r := range "devon" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if m.phase != phaseWizard || m.step() != "theme" {
		t.Fatalf("phase/step after handle = %q/%q, want wizard/theme", m.phase, m.step())
	}

	// Wizard: theme (pick Midnight, index 1), accent (pick Purple), density
	// (pick Compact), keyboard (complete all drills), status (pick Away).
	m = Key(m, "2") // Midnight
	m = Key(m, "enter")
	if m.step() != "accent" {
		t.Fatalf("step after theme = %q, want accent", m.step())
	}
	m = Key(m, "4") // Purple (accentOpts: auto,cyan,green,purple,orange,magenta)
	m = Key(m, "enter")
	if m.step() != "density" {
		t.Fatalf("step after accent = %q, want density", m.step())
	}
	m = Key(m, "1") // Compact
	m = Key(m, "enter")
	if m.step() != "keyboard" {
		t.Fatalf("step after density = %q, want keyboard", m.step())
	}

	// keyboard: complete all four drills for real, then continue.
	m = Key(m, "j")
	m = Key(m, "k")
	m = update(m, advanceDrillMsg{})
	m = Key(m, "i")
	m = Key(m, "hi")
	m = Key(m, "enter")
	m = update(m, advanceDrillMsg{})
	m = Key(m, "t")
	m = update(m, advanceDrillMsg{})
	m = Key(m, "ctrl+k")
	m = Key(m, "esc")
	m = update(m, advanceDrillMsg{})
	if !m.kbDone {
		t.Fatal("all four drills should be complete")
	}
	m = Key(m, "enter")
	if m.step() != "status" {
		t.Fatalf("step after keyboard = %q, want status", m.step())
	}
	m = Key(m, "2") // Away
	m = Key(m, "enter")
	if m.phase != phaseLaunch {
		t.Fatalf("phase after status = %q, want launch", m.phase)
	}

	// Launch: enter finishes and persists prefs.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("finishing should return a command")
	}
	msg := cmd()
	fin, ok := msg.(FinishedMsg)
	if !ok {
		t.Fatalf("finish message type = %T, want FinishedMsg", msg)
	}
	if fin.Prefs.Handle != "devon" || fin.Prefs.Theme != "midnight" || fin.Prefs.Accent != "purple" ||
		fin.Prefs.Density != "compact" || fin.Prefs.Status != "away" || !fin.Prefs.Onboarded {
		t.Fatalf("finished prefs = %+v, want handle=devon theme=midnight accent=purple density=compact status=away onboarded=true", fin.Prefs)
	}
	// And the write actually reached disk (finish() calls config.Save
	// synchronously, not inside the returned cmd).
	loaded, ok := config.Load()
	if !ok || loaded.Theme != "midnight" || loaded.Handle != "devon" {
		t.Fatalf("prefs not persisted to disk: ok=%v loaded=%+v", ok, loaded)
	}
}

// TestWorkspaceNameSlug is the independent table for the slugification rule
// finish/oauthDoneMsg rely on: lowercase, spaces → dashes, empty → "default".
func TestWorkspaceNameSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Acme Corp", "acme-corp"},
		{"", "default"},
		{"  spaced  out  ", "spaced--out"}, // TrimSpace first, then every internal space → dash
		{"already-lower", "already-lower"},
	}
	for _, c := range cases {
		if got := workspaceName(c.in); got != c.want {
			t.Errorf("workspaceName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestOAuthErrorSurfacesAndDismisses: a failed oauthDoneMsg records the error
// on the oauth screen; any key dismisses it back to the auth menu instead of
// silently advancing into a broken identity screen.
func TestOAuthErrorSurfacesAndDismisses(t *testing.T) {
	m := Goto(sized(), phaseOAuth)
	m.oauthRunning = true
	next, _ := m.Update(oauthDoneMsg{err: fmt.Errorf("access_denied")})
	m = next
	if m.oauthRunning {
		t.Error("a finished oauth attempt must clear oauthRunning even on error")
	}
	if m.oauthErr == "" {
		t.Fatal("failed oauth should record oauthErr")
	}
	view := Dump(m, 100, 30)
	if !strings.Contains(view, "sign-in failed") {
		t.Errorf("oauth screen should show the failure, got:\n%s", view)
	}
	m = Key(m, "x") // any key dismisses the error
	if m.oauthErr != "" || m.phase != phaseAuth {
		t.Errorf("dismissing the error should clear it and return to auth: err=%q phase=%q", m.oauthErr, m.phase)
	}
}

// TestOAuthEscCancelsWhileRunning: esc during the real browser wait cancels
// back to the auth menu without waiting for a callback that may never come.
func TestOAuthEscCancelsWhileRunning(t *testing.T) {
	m := Goto(sized(), phaseOAuth)
	m.oauthRunning = true
	m = Key(m, "esc")
	if m.oauthRunning || m.phase != phaseAuth {
		t.Errorf("esc should cancel the wait: running=%v phase=%q", m.oauthRunning, m.phase)
	}
}

// ── rendering: auth menu selection marker ────────────────────────────────

// TestAuthSelectionMarkerMovesWithCursor: the "↵" enter glyph appears on
// exactly the selected option's row (not as a static decoration), so moving
// the cursor must move which row shows it — a real regression class (the
// glyph baked into the wrong row, or shown on none/all rows) fails this.
func TestAuthSelectionMarkerMovesWithCursor(t *testing.T) {
	m := Goto(sized(), phaseAuth)
	stageOf := func(mm Model) string {
		lines := strings.Split(ansi.Strip(Dump(mm, 96, 30)), "\n")
		// drop the status bar (last line) — its hints also contain "↵".
		return strings.Join(lines[:len(lines)-1], "\n")
	}
	rowContaining := func(body, label string) string {
		for _, l := range strings.Split(body, "\n") {
			if strings.Contains(l, label) {
				return l
			}
		}
		return ""
	}

	body := stageOf(m)
	slackRow := rowContaining(body, "Sign in with Slack")
	if slackRow == "" {
		t.Fatal("auth screen should list 'Sign in with Slack'")
	}
	if !strings.Contains(slackRow, "↵") {
		t.Errorf("selected row (Sign in with Slack) should carry the enter marker, got %q", slackRow)
	}
	tokenRow := rowContaining(body, "Paste an auth token")
	if strings.Contains(tokenRow, "↵") {
		t.Errorf("unselected row (Paste an auth token) should not carry the enter marker, got %q", tokenRow)
	}

	m = Key(m, "j") // move selection down
	body = stageOf(m)
	slackRow = rowContaining(body, "Sign in with Slack")
	tokenRow = rowContaining(body, "Paste an auth token")
	if strings.Contains(slackRow, "↵") {
		t.Errorf("after moving down, the old row should lose the marker, got %q", slackRow)
	}
	if !strings.Contains(tokenRow, "↵") {
		t.Errorf("after moving down, the new row should carry the marker, got %q", tokenRow)
	}
}

// TestRenderTokenFormFocusedFieldCursor: all three token fields are labeled,
// and the cursor glyph appears only on the currently-focused field.
func TestRenderTokenFormFocusedFieldCursor(t *testing.T) {
	m := Goto(sized(), phaseToken)
	view := ansi.Strip(Dump(m, 100, 30))
	for _, want := range []string{"user token (xoxp):", "app token  (xapp):", "bot token  (xoxb):"} {
		if !strings.Contains(view, want) {
			t.Errorf("token form missing field label %q", want)
		}
	}
	// Use the exact field-row labels (with their trailing colon) — the intro
	// sentence above also contains the bare words "user token"/"app"/"bot
	// tokens", which would otherwise false-match the wrong line.
	rowWithCursor := func(view, label string) bool {
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, label) {
				return strings.Contains(l, "▋")
			}
		}
		return false
	}
	if !rowWithCursor(view, "user token (xoxp):") {
		t.Error("field 0 (user token) should be focused by default and show the cursor")
	}
	if rowWithCursor(view, "app token  (xapp):") {
		t.Error("field 1 (app token) should not show the cursor while unfocused")
	}

	// tab moves focus to the next field.
	m = Key(m, "tab")
	view = ansi.Strip(Dump(m, 100, 30))
	if rowWithCursor(view, "user token (xoxp):") {
		t.Error("after tab, field 0 should no longer show the cursor")
	}
	if !rowWithCursor(view, "app token  (xapp):") {
		t.Error("after tab, field 1 (app token) should show the cursor")
	}
}

// ── wizard card grids ────────────────────────────────────────────────────

// TestWizardCardGridsListAllOptionsWithOneSelected walks each option-driven
// step and checks: every option's name is rendered, and exactly one card
// carries the "◉" selection marker (never zero, never more than one) —
// before and after moving the cursor. A regression that stops clearing the
// old marker (or never sets a new one) fails this.
func TestWizardCardGridsListAllOptionsWithOneSelected(t *testing.T) {
	cases := []struct {
		step  string
		names []string
	}{
		{"theme", []string{"Charcoal", "Midnight", "Phosphor", "Solarized", "Paper"}},
		{"accent", []string{"Auto", "Cyan", "Green", "Purple", "Orange", "Magenta"}},
		{"density", []string{"Compact", "Comfortable"}},
		// status is deliberately excluded here: statusRow marks selection only
		// via border color, not a textual "◉" glyph — see
		// TestStatusStepSelectionBorderColor below.
	}
	for _, c := range cases {
		t.Run(c.step, func(t *testing.T) {
			m := Goto(sized(), "wizard:"+c.step)
			view := cardBody(ansi.Strip(Dump(m, 104, 30)))
			for _, name := range c.names {
				if !strings.Contains(view, name) {
					t.Errorf("%s step missing option %q", c.step, name)
				}
			}
			if n := strings.Count(view, "◉"); n != 1 {
				t.Errorf("%s step: want exactly one selection marker, got %d in body:\n%s", c.step, n, view)
			}
			m = Key(m, "j")
			view = cardBody(ansi.Strip(Dump(m, 104, 30)))
			if n := strings.Count(view, "◉"); n != 1 {
				t.Errorf("%s step after moving: want exactly one selection marker, got %d in body:\n%s", c.step, n, view)
			}
		})
	}
}

// TestStatusStepSelectionBorderColor: unlike the card-based steps, the status
// list marks its selected row only by border color (p.Accent vs p.Border),
// not a textual glyph. Checked against the literal rendered escape for
// p.Accent (built by rendering a throwaway string with that exact style, not
// hand-parsed hex) so the assertion can't pass by mirroring statusRow's own
// color-selection logic.
func TestStatusStepSelectionBorderColor(t *testing.T) {
	m := sized()
	p := m.pal()
	accentOpen := escapeOpen(lipgloss.NewStyle().Foreground(p.Accent))
	selRow := m.statusRow(p, 0, "online", "Active", "available and reachable", true, 40)
	unselRow := m.statusRow(p, 0, "online", "Active", "available and reachable", false, 40)
	if !strings.Contains(selRow, accentOpen) {
		t.Errorf("selected status row should use the accent border color, got %q", selRow)
	}
	if strings.Contains(unselRow, accentOpen) {
		t.Errorf("unselected status row should not use the accent border color, got %q", unselRow)
	}
}

// escapeOpen renders a throwaway string with the given style and returns just
// the opening SGR sequence — used to check a specific style was applied
// without hand-computing truecolor byte values (see TestBootColorDistinctStyling).
func escapeOpen(s lipgloss.Style) string {
	probe := s.Render("z")
	return probe[:strings.IndexByte(probe, 'z')]
}

// cardBody strips the frame's top border line from a stripped view — that
// line embeds the wizard step rail, which renders its own "◉" markers for
// completed/current steps and would otherwise be counted alongside the
// card-grid's own selection marker.
func cardBody(view string) string {
	var out []string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "┌─ setup") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// ── trainer view ─────────────────────────────────────────────────────────

// TestTrainerViewDrillTransition: entering at drill 2 shows drills 0-1 as
// done, drill 2's instruction, and completing drill 2 for real (pressing t)
// advances the rendered instruction to drill 3's text.
func TestTrainerViewDrillTransition(t *testing.T) {
	m := Goto(sized(), "wizard:keyboard:2")
	view := ansi.Strip(Dump(m, 104, 30))
	if !strings.Contains(view, "Select a message and press") {
		t.Fatalf("drill 2 instruction not shown:\n%s", view)
	}
	if strings.Contains(view, "Press [⌃K]") {
		t.Fatal("drill 3 instruction should not be visible yet")
	}
	// drills 0 and 1 must show as done (◉ mark in the trainer rail), drill 2
	// current (▸). Exclude the wizard step rail (frame top border), which
	// renders its own "◉" markers for completed/current wizard steps.
	body := cardBody(view)
	if n := strings.Count(body, "◉"); n != 2 {
		t.Errorf("want 2 completed-drill marks (rail), got %d in:\n%s", n, body)
	}

	m, cmd := KeyC(m, "t") // completes the threads drill for real
	if !m.trainer.done[2] {
		t.Fatal("pressing t should complete the threads drill")
	}
	m = Pump(m, cmd) // auto-advance to drill 3
	if m.trainer.drill != 3 {
		t.Fatalf("drill after threads = %d, want 3", m.trainer.drill)
	}
	view = ansi.Strip(Dump(m, 104, 30))
	if !strings.Contains(view, "Press [⌃K]") {
		t.Errorf("drill 3 instruction should now be visible:\n%s", view)
	}
	if strings.Contains(view, "Select a message and press") {
		t.Errorf("drill 2 instruction should no longer be visible:\n%s", view)
	}
}

// ── typewriter ───────────────────────────────────────────────────────────

// TestTypewriterProgressiveReveal steps the boot typewriter tick-by-tick
// (via the real onTick path, not a synthetic jump) and checks the reveal is
// gated on the exact character count of the first line — not one tick early
// or late — then confirms a later line is still hidden.
func TestTypewriterProgressiveReveal(t *testing.T) {
	m := sized() // phaseBoot
	first := bootLines()[0]
	if first.fill {
		t.Fatal("test assumes the first boot line has no dotted fill")
	}
	n := len([]rune(first.text))

	// One tick short of the first line's full length: it must be partial.
	mm := m
	for i := 0; i < n-1; i++ {
		mm = Tick(mm)
	}
	view := ansi.Strip(Dump(mm, 100, 30))
	if strings.Contains(view, first.text) {
		t.Fatalf("first line fully visible one tick too early:\n%s", view)
	}

	// Exactly n ticks: the first line is complete; a much later line is not.
	mm = Tick(mm)
	view = ansi.Strip(Dump(mm, 100, 30))
	if !strings.Contains(view, first.text) {
		t.Fatalf("first line should be fully visible after %d ticks:\n%s", n, view)
	}
	if strings.Contains(view, "mounting message store") {
		t.Fatalf("a later boot line should not be visible yet:\n%s", view)
	}
}

// TestTypewriterFastForward: fast-forwarding the boot reveals every line,
// including the last ("session ready…").
func TestTypewriterFastForward(t *testing.T) {
	m := Key(sized(), " ")
	if !m.boot.done {
		t.Fatal("space should fast-forward the boot typewriter")
	}
	view := ansi.Strip(Dump(m, 100, 30))
	last := bootLines()[len(bootLines())-1]
	if !strings.Contains(view, last.text) {
		t.Errorf("fast-forward should reveal the final boot line, got:\n%s", view)
	}
}

// TestBootColorDistinctStyling ties each typewriter class to the specific
// palette field it must use, by independently building a style from that
// named field and comparing the rendered escape sequences (rather than
// hand-computing the expected truecolor SGR bytes — lipgloss/termenv route
// hex colors through a colorful round-trip that rounds a channel by ±1, so a
// hand-parsed hex doesn't reliably match its own rendered output). A
// class→field remap (e.g. "ok" accidentally using p.Accent instead of
// p.Green) is still caught, since the fields have different hexes.
func TestBootColorDistinctStyling(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	render := func(c lipgloss.Color) string { return lipgloss.NewStyle().Foreground(c).Render("x") }

	if got, want := bootColor(p, "ok").Render("x"), render(p.Green); got != want {
		t.Errorf("ok-class should render as p.Green: got %q want %q", got, want)
	}
	if got, want := bootColor(p, "dim").Render("x"), render(p.Dim); got != want {
		t.Errorf("dim-class should render as p.Dim: got %q want %q", got, want)
	}
	if got, want := bootColor(p, "accent").Render("x"), render(p.Accent); got != want {
		t.Errorf("accent-class should render as p.Accent: got %q want %q", got, want)
	}
	if got, want := bootColor(p, "fill").Render("x"), render(p.Dim2); got != want {
		t.Errorf("fill-class should render as p.Dim2: got %q want %q", got, want)
	}
	// And the classes must actually differ from each other (guards a copy-paste
	// that maps two distinct classes onto the same field).
	if bootColor(p, "ok").Render("x") == bootColor(p, "dim").Render("x") {
		t.Error("ok and dim classes rendered identically")
	}
}

// TestOAuthScreenShowsRealRedirectURI drives the real sign-in-with-Slack path
// (startOAuth, as chooseAuth calls it once an app is configured) and checks
// the waiting screen names the actual redirect URI auth.RedirectURIs()
// computes — an independent source. A hardcoded copy drifting from the real
// value would silently break sign-in; fast-forwarding the typewriter and
// reading the rendered frame catches that without executing oauthCmd (no
// network).
func TestOAuthScreenShowsRealRedirectURI(t *testing.T) {
	m, _ := sized().startOAuth(config.OAuthCreds{ClientID: "123.456"})
	m.oauth.fastForward()
	view := ansi.Strip(Dump(m, 100, 30))
	want := auth.RedirectURIs()[0]
	if !strings.Contains(view, want) {
		t.Errorf("oauth screen should show the real redirect URI %q, got:\n%s", want, view)
	}
}

// ── pure layout helpers ──────────────────────────────────────────────────

// TestDistributeWidths: widths sum exactly to the total and never differ by
// more than one column — the invariant swatchStrip/cardGrid rely on to avoid
// a ragged card edge.
func TestDistributeWidths(t *testing.T) {
	cases := []struct {
		total, n int
		want     []int
	}{
		{10, 3, []int{4, 3, 3}},
		{9, 3, []int{3, 3, 3}},
		{1, 4, []int{1, 0, 0, 0}},
	}
	for _, c := range cases {
		got := distribute(c.total, c.n)
		sum := 0
		max, min := got[0], got[0]
		for _, v := range got {
			sum += v
			if v > max {
				max = v
			}
			if v < min {
				min = v
			}
		}
		if sum != c.total {
			t.Errorf("distribute(%d,%d) sums to %d, want %d", c.total, c.n, sum, c.total)
		}
		if max-min > 1 {
			t.Errorf("distribute(%d,%d) = %v, spread > 1", c.total, c.n, got)
		}
		if len(c.want) > 0 {
			for i, w := range c.want {
				if got[i] != w {
					t.Errorf("distribute(%d,%d)[%d] = %d, want %d", c.total, c.n, i, got[i], w)
				}
			}
		}
	}
}

// TestWrapStyledRespectsWidth: no wrapped line exceeds the requested width,
// and the styling escape survives (wrapStyled must not degrade to plain text).
func TestWrapStyledRespectsWidth(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	style := lipgloss.NewStyle().Foreground(p.Dim)
	text := "The whole client follows your choice as you preview each theme live."
	out := wrapStyled(style, text, 24)
	for _, l := range strings.Split(out, "\n") {
		if w := lipgloss.Width(l); w > 24 {
			t.Errorf("wrapped line exceeds width 24: %q (width %d)", l, w)
		}
	}
	// The opening SGR sequence lipgloss emits for this exact style (computed by
	// rendering a throwaway string with the same style, not hand-parsed hex —
	// see TestBootColorDistinctStyling for why) must still open the output.
	probe := style.Render("x")
	openSeq := probe[:strings.IndexByte(probe, 'x')]
	if !strings.Contains(out, openSeq) {
		t.Errorf("wrapped text should retain its foreground styling; want prefix %q in %q", openSeq, out)
	}
}

// ── Goto coverage across documented phase targets ───────────────────────

// TestGotoAllDocumentedTargets exercises every phase string --dump-ob
// documents, checking a phase-distinctive substring lands in the frame —
// this guards the dev tool from silently landing on the wrong screen.
func TestGotoAllDocumentedTargets(t *testing.T) {
	cases := []struct {
		target, want string
	}{
		{"boot", "monospace-labs"},
		{"auth", "Sign in with Slack"},
		{"appsetup", "Client ID"},
		{"oauth", "sign in with Slack"},
		{"wizard:theme", "Choose a theme"},
		{"wizard:accent", "Accent color"},
		{"wizard:density", "Message density"},
		{"wizard:keyboard", "Learn the keys"},
		{"wizard:status", "Set your presence"},
		{"launch", "session configured"},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			m := Goto(sized(), c.target)
			view := ansi.Strip(Dump(m, 100, 30))
			if !strings.Contains(view, c.want) {
				t.Errorf("Goto(%q) view missing %q, got:\n%s", c.target, c.want, view)
			}
		})
	}
}

// TestOnboardingInitReturnsTick: Init's returned command must actually yield
// a tickMsg — the boot animation never starts if this regresses.
func TestOnboardingInitReturnsTick(t *testing.T) {
	m := New()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a command")
	}
	if _, ok := cmd().(tickMsg); !ok {
		t.Errorf("Init's command should yield tickMsg, got %T", cmd())
	}
}

package onboarding

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/config"
)

func sized() Model { return WithSize(New(), 100, 30) }

func update(m Model, msg tea.Msg) Model { next, _ := m.Update(msg); return next }

func TestBootFastForwardThenAuth(t *testing.T) {
	m := sized()
	m = Key(m, " ") // any key fast-forwards boot
	if !m.boot.done {
		t.Fatal("a key during boot should fast-forward it")
	}
	m = update(m, bootAdvanceMsg{})
	if m.phase != phaseAuth {
		t.Errorf("phase = %q, want auth", m.phase)
	}
}

func TestAuthGuestToIdentity(t *testing.T) {
	m := sized()
	m.phase = phaseAuth
	m = Key(m, "4") // continue as guest
	if m.phase != phaseIdentity {
		t.Errorf("guest auth → phase %q, want identity", m.phase)
	}
	if m.provider != "guest" {
		t.Errorf("provider = %q, want guest", m.provider)
	}
}

func TestTokenFlow(t *testing.T) {
	m := sized()
	m.phase = phaseAuth
	m = Key(m, "3") // paste a token
	if m.phase != phaseToken {
		t.Fatalf("phase = %q, want token", m.phase)
	}
	m = Key(m, "ent") // too short
	m = Key(m, "enter")
	if m.phase != phaseToken {
		t.Error("token under 4 chars should not advance")
	}
	m = Key(m, "xoxp-123")
	m = Key(m, "enter")
	if m.phase != phaseIdentity {
		t.Errorf("valid token → phase %q, want identity", m.phase)
	}
}

func TestIdentityToWizard(t *testing.T) {
	m := sized()
	m.phase = phaseIdentity
	m.handle.Focus()
	m = Key(m, "enter") // empty handle: no advance
	if m.phase != phaseIdentity {
		t.Error("empty handle should not advance")
	}
	m = Key(m, "devon")
	m = Key(m, "enter")
	if m.phase != phaseWizard || m.step() != "theme" {
		t.Errorf("phase/step = %q/%q, want wizard/theme", m.phase, m.step())
	}
}

func TestWizardThemePreview(t *testing.T) {
	m := sized()
	m.phase = phaseWizard
	m = m.syncOpt()
	start := m.themeName
	m = Key(m, "j")
	if m.themeName == start {
		t.Errorf("j should preview a different theme, still %q", m.themeName)
	}
}

func TestWizardKeyboardGate(t *testing.T) {
	m := sized()
	m.phase = phaseWizard
	m.stepIndex = 3 // keyboard
	if m.step() != "keyboard" {
		t.Fatalf("step = %q, want keyboard", m.step())
	}
	m = Key(m, "enter") // should NOT advance — drills incomplete
	if m.stepIndex != 3 {
		t.Errorf("keyboard step advanced before drills complete (stepIndex=%d)", m.stepIndex)
	}
}

func TestKeyboardStepEscGoesBack(t *testing.T) {
	m := sized()
	m.phase = phaseWizard
	m.stepIndex = 3 // keyboard
	m = Key(m, "esc")
	if m.stepIndex != 2 {
		t.Fatalf("esc on keyboard step should go back to density (stepIndex=%d)", m.stepIndex)
	}
}

func TestKeyboardStepEscCancelsInsertBeforeBack(t *testing.T) {
	m := sized()
	m.phase = phaseWizard
	m.stepIndex = 3
	// advance to the compose drill and enter insert
	m = Key(m, "j")
	m = Key(m, "k")
	m = update(m, advanceDrillMsg{}) // → drill 1
	m = Key(m, "i")                  // enter insert
	if !(m.trainer.mode == "insert" && m.trainer.drill == 1) {
		t.Fatalf("expected compose-insert state, got mode=%q drill=%d", m.trainer.mode, m.trainer.drill)
	}
	m = Key(m, "esc") // should cancel insert, NOT navigate back
	if m.stepIndex != 3 {
		t.Errorf("esc during compose-insert should not leave the keyboard step (stepIndex=%d)", m.stepIndex)
	}
	if m.trainer.mode != "normal" {
		t.Errorf("esc should cancel insert, mode=%q", m.trainer.mode)
	}
}

// TestTrainerRealFlow drives the trainer through the actual command path —
// executing the 900ms auto-advance ticks — to prove every drill (especially
// threads) completes in the live flow, not just via injected messages.
func TestTrainerRealFlow(t *testing.T) {
	m := sized()
	m.phase = phaseWizard
	m.stepIndex = 3
	var c tea.Cmd

	// navigate: j then k
	m, _ = KeyC(m, "j")
	m, c = KeyC(m, "k")
	if !m.trainer.done[0] {
		t.Fatal("navigate (j+k) did not complete")
	}
	m = Pump(m, c) // auto-advance → compose
	if m.trainer.drill != 1 {
		t.Fatalf("after navigate, drill=%d want 1", m.trainer.drill)
	}

	// compose: i, type, enter
	m, _ = KeyC(m, "i")
	m, _ = KeyC(m, "x")
	m, c = KeyC(m, "enter")
	if !m.trainer.done[1] {
		t.Fatal("compose did not complete")
	}
	m = Pump(m, c) // → threads
	if m.trainer.drill != 2 {
		t.Fatalf("after compose, drill=%d want 2", m.trainer.drill)
	}

	// threads: press t  ← the reported-broken step
	m, c = KeyC(m, "t")
	if !m.trainer.done[2] {
		t.Fatal("pressing t did not complete the threads drill")
	}
	m = Pump(m, c) // → palette
	if m.trainer.drill != 3 {
		t.Fatalf("after threads, drill=%d want 3", m.trainer.drill)
	}

	// palette: ctrl+k then esc
	m, _ = KeyC(m, "ctrl+k")
	m, c = KeyC(m, "esc")
	if !m.trainer.done[3] {
		t.Fatal("palette did not complete")
	}
	m = Pump(m, c)
	if !m.kbDone {
		t.Fatal("kbDone should be true after all four drills")
	}
}

func TestTrainerCompletesAllDrills(t *testing.T) {
	m := sized()
	m.phase = phaseWizard
	m.stepIndex = 3

	// drill 0: navigate (down then up)
	m = Key(m, "j")
	m = Key(m, "k")
	if !m.trainer.done[0] {
		t.Fatal("nav drill not marked done")
	}
	m = update(m, advanceDrillMsg{})

	// drill 1: compose
	m = Key(m, "i")
	m = Key(m, "hi")
	m = Key(m, "enter")
	if !m.trainer.done[1] {
		t.Fatal("compose drill not marked done")
	}
	m = update(m, advanceDrillMsg{})

	// drill 2: threads
	m = Key(m, "t")
	if !m.trainer.done[2] {
		t.Fatal("thread drill not marked done")
	}
	m = update(m, advanceDrillMsg{})

	// drill 3: palette
	m = Key(m, "ctrl+k")
	m = Key(m, "esc")
	if !m.trainer.done[3] {
		t.Fatal("palette drill not marked done")
	}
	m = update(m, advanceDrillMsg{})

	if !m.kbDone {
		t.Error("kbDone should be true after all drills")
	}
	m = Key(m, "enter") // now allowed to continue
	if m.stepIndex != 4 {
		t.Errorf("after kbDone, enter should advance to status (stepIndex=%d)", m.stepIndex)
	}
}

func TestFinishEmitsPrefs(t *testing.T) {
	isolateConfigDir(t)
	m := Goto(WithSize(New(), 100, 30), phaseLaunch)
	m.themeName = "midnight"
	m.accent = "purple"
	m.density = "compact"
	m.status = "away"
	_, cmd := m.finish()
	if cmd == nil {
		t.Fatal("finish should return a command")
	}
	msg := cmd()
	fin, ok := msg.(FinishedMsg)
	if !ok {
		t.Fatalf("finish msg type = %T, want FinishedMsg", msg)
	}
	if fin.Prefs.Theme != "midnight" || fin.Prefs.Handle != "devon" || !fin.Prefs.Onboarded {
		t.Errorf("prefs = %+v", fin.Prefs)
	}
}

// isolateConfigDir points config.Dir() at a temp dir for the duration of a test.
// Overriding only HOME is not enough: config.Dir() checks XDG_CONFIG_HOME first,
// so on any desktop that sets it (most Linux distros) the test would read and
// write the real ~/.config/slack-tui.
func isolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// With no Slack app configured — the normal state for a fresh install, since
// slack-tui isn't distributed through Slack — "Sign in with Slack" must set the
// app up in place. It used to dump the user on the paste-a-token screen and
// tell them to quit and run a CLI command.
func TestSignInWithoutAppOffersSetup(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("SLACK_CLIENT_ID", "")
	t.Setenv("SLACK_CLIENT_SECRET", "")

	m := Key(Goto(WithSize(New(), 96, 30), phaseAuth), "1")
	if m.phase != phaseAppSetup {
		t.Fatalf("phase = %q, want %q", m.phase, phaseAppSetup)
	}
	view := Dump(m, 96, 30)
	for _, want := range []string{"Client ID", "app manifest", "api.slack.com/apps"} {
		if !strings.Contains(view, want) {
			t.Errorf("app-setup screen missing %q", want)
		}
	}
}

// A bad client ID must be reported in place and never written to disk — the
// paste is one keystroke away from the App ID sitting just above it.
func TestAppSetupRejectsBadClientID(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("SLACK_CLIENT_ID", "")
	t.Setenv("SLACK_CLIENT_SECRET", "")

	m := Key(Goto(WithSize(New(), 96, 30), phaseAuth), "1")
	for _, k := range strings.Split("A,0,B,8,K,U,L,B,K,8,W,enter", ",") {
		m = Key(m, k)
	}
	if m.phase != phaseAppSetup {
		t.Errorf("should stay on the setup screen, got %q", m.phase)
	}
	if !strings.Contains(m.appSetupNote, "App ID") {
		t.Errorf("note = %q, want the App ID hint", m.appSetupNote)
	}
	if got := config.LoadOAuthCreds(); got.ClientID != "" {
		t.Errorf("a rejected id must not be saved, got %q", got.ClientID)
	}
}

// Rendering must stay free of side effects: the clipboard write and the browser
// launch hang off explicit keys, so tests and --dump-ob never clobber the user's
// clipboard or open a window.
func TestAppSetupRenderHasNoSideEffects(t *testing.T) {
	isolateConfigDir(t)
	prev := AppManifest
	AppManifest = "" // as in any build that didn't inject it
	t.Cleanup(func() { AppManifest = prev })
	m := Goto(WithSize(New(), 96, 30), phaseAppSetup)
	_ = Dump(m, 96, 30)
	if m.appSetupNote != "" {
		t.Errorf("rendering set a note (%q) — something ran that shouldn't have", m.appSetupNote)
	}
	// …and the copy key degrades gracefully when no manifest was injected.
	m = Key(m, "ctrl+y")
	if !strings.Contains(m.appSetupNote, "unavailable") {
		t.Errorf("note = %q, want the unavailable-manifest message", m.appSetupNote)
	}
}

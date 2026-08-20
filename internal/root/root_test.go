package root

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/app"
	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/onboarding"
	"github.com/kurenn/slack-tui/internal/source"
)

// smIsolateConfigDir points config.Dir() at a fresh temp dir for the
// duration of a test — root.New()'s whole job is reading config.Load() to
// decide onboarding vs. loading, so every test here needs its own directory
// rather than sharing whatever prefs.json a sibling test in this package
// left behind.
func smIsolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// With no saved prefs, New() must route to onboarding — proven by comparing
// against onboarding.New()'s own View() (an independent oracle), not by
// checking the mode enum directly. If New() ever defaulted to modeLoading
// instead, this would render the "connecting…" string and fail the
// comparison.
func TestNewRoutesToOnboardingWhenNotOnboarded(t *testing.T) {
	smIsolateConfigDir(t)
	m := New()

	want := onboarding.New().View()
	if got := m.View(); got != want {
		t.Errorf("root.New() with no saved prefs should render the onboarding boot frame.\ngot:  %q\nwant: %q", got, want)
	}
}

// Once prefs.json says onboarded, New() must skip onboarding and go straight
// to the loading screen — this is the hand-off seam onboarding.FinishedMsg
// depends on.
func TestNewRoutesToLoadingWhenOnboarded(t *testing.T) {
	smIsolateConfigDir(t)
	prefs := config.Defaults()
	prefs.Onboarded = true
	if err := config.Save(prefs); err != nil {
		t.Fatal(err)
	}

	m := New()
	if got := m.View(); !strings.Contains(got, "connecting to slack") {
		t.Errorf("onboarded prefs should route to the loading screen, got View() = %q", got)
	}
}

// Once the app is ready, View() must render it at the size root already
// received via WindowSizeMsg — not the zero size the app model started
// with. A full-width first line is the observable proof: a wrong (unapplied)
// size would either error rendering or produce a narrower/wider frame.
func TestUpdateAppliesRememberedSizeWhenAppBecomesReady(t *testing.T) {
	m := Model{mode: modeLoading}
	const w, h = 100, 30

	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(Model)

	readyApp := app.NewWith(source.NewMock(), config.Defaults())
	next, cmd := m.Update(appReadyMsg{app: readyApp})
	m = next.(Model)
	if cmd == nil {
		t.Error("appReadyMsg should return a non-nil init cmd from the app")
	}

	lines := strings.Split(m.View(), "\n")
	if len(lines) == 0 {
		t.Fatal("app view produced no lines")
	}
	if got := lipgloss.Width(lines[0]); got != w {
		t.Errorf("first rendered line width = %d, want the remembered window width %d", got, w)
	}
}

// onboarding.FinishedMsg must switch to the loading screen and return a
// non-nil cmd (loadApp) — this is the seam that re-reads the prefs
// onboarding just wrote and builds the real app from them.
func TestFinishedMsgSwitchesToLoading(t *testing.T) {
	m := Model{mode: modeOnboarding, ob: onboarding.New()}
	next, cmd := m.Update(onboarding.FinishedMsg{Prefs: config.Defaults()})
	m = next.(Model)

	if !strings.Contains(m.View(), "connecting to slack") {
		t.Errorf("expected the loading view after FinishedMsg, got %q", m.View())
	}
	if cmd == nil {
		t.Error("FinishedMsg should return a non-nil cmd (loadApp)")
	}
}

// app.ReloadMsg (a workspace switch) must tear the live app down and go back
// to loading with a non-nil cmd — and must not panic doing so, proving
// Shutdown is safe to call on a mock-backed app.
func TestReloadMsgReturnsToLoading(t *testing.T) {
	smIsolateConfigDir(t)
	m := Model{mode: modeLoading}
	readyApp := app.NewWith(source.NewMock(), config.Defaults())
	next, _ := m.Update(appReadyMsg{app: readyApp})
	m = next.(Model)
	if strings.Contains(m.View(), "connecting to slack") {
		t.Fatal("test setup failed: app should be live (not loading) before ReloadMsg")
	}

	next, cmd := m.Update(app.ReloadMsg{})
	m = next.(Model)
	if !strings.Contains(m.View(), "connecting to slack") {
		t.Errorf("expected the loading view after ReloadMsg, got %q", m.View())
	}
	if cmd == nil {
		t.Error("ReloadMsg should return a non-nil cmd (loadApp)")
	}
}

// q and ctrl+c on the loading screen must quit outright — there's no app
// model yet to hand the keypress to, so this is root's own responsibility.
func TestLoadingScreenQuitKeys(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyCtrlC},
	} {
		m := Model{mode: modeLoading}
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("key %v should return tea.Quit, got nil cmd", key)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("key %v should return a cmd yielding tea.QuitMsg", key)
		}
	}
}

// In loading mode, Init must return loadApp — and running it (as bubbletea
// would) must actually yield an appReadyMsg carrying a built app model. This
// is the network-off-the-UI-thread contract root exists to provide.
func TestInitInLoadingModeReturnsLoadAppCmd(t *testing.T) {
	smIsolateConfigDir(t) // no real tokens/prefs must leak into app.New()
	m := Model{mode: modeLoading}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init in loading mode should return a non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(appReadyMsg); !ok {
		t.Fatalf("Init's cmd should yield appReadyMsg, got %T", msg)
	}
}

// In onboarding mode, Init must delegate to the onboarding model's own Init
// (the typewriter's tick cmd) rather than jumping straight to loadApp.
func TestInitInOnboardingModeDelegatesToOnboarding(t *testing.T) {
	m := Model{mode: modeOnboarding, ob: onboarding.New()}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init in onboarding mode returned no cmd")
	}
	// Running the cmd is the point: asserting only that it is non-nil passes
	// for a regression that returns loadApp here, which would start loading the
	// workspace — authenticated work — while onboarding is still on screen.
	msg := cmd()
	if _, bad := msg.(appReadyMsg); bad {
		t.Fatal("Init delegated to loadApp during onboarding — the app must not load until onboarding finishes")
	}
	want := onboarding.New().Init()()
	if got, wantT := reflect.TypeOf(msg), reflect.TypeOf(want); got != wantT {
		t.Errorf("Init yielded %v, want onboarding's own init message %v", got, wantT)
	}
}

// Any other key on the loading screen must be a no-op (no cmd, stay loading)
// — there's nothing else it can do until the app is ready.
func TestLoadingScreenOtherKeyIsNoop(t *testing.T) {
	m := Model{mode: modeLoading}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if cmd != nil {
		t.Error("a non-quit key on the loading screen should return a nil cmd")
	}
	if !strings.Contains(m.View(), "connecting to slack") {
		t.Error("a non-quit key should leave the loading screen showing")
	}
}

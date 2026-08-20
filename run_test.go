package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/kurenn/slack-tui/internal/config"
)

// smIsolateConfigDir points config.Dir()/theme state lookups at fresh temp
// dirs, so run()'s dev flags (--dump, --dump-ob) that build a real app.New()
// or onboarding.New() never touch this machine's real config, tokens, or
// desktop theme. testenv.Pin already does this once for the whole package
// (TestMain), but --workspace tests need their own directory per-test.
func smIsolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_STATE_HOME", dir)
}

// smCaptureStdout runs fn with stdout redirected and returns what it printed.
func smCaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w

	// A dumped frame in truecolor easily exceeds the OS pipe buffer (64KB on
	// Linux): every cell can carry its own SGR sequence. Draining must run
	// concurrently with fn(), or a write past the buffer size blocks forever
	// with nothing reading the other end — a real deadlock this package hit
	// during development, not a hypothetical.
	outCh := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		outCh <- string(b)
	}()

	fn()
	w.Close()
	os.Stdout = prev
	return <-outCh
}

// versionString must report the goreleaser-stamped version verbatim when one
// is set, without consulting build info at all.
func TestVersionStringStamped(t *testing.T) {
	prev := version
	version = "v1.2.3"
	defer func() { version = prev }()

	if got := versionString(); got != "v1.2.3" {
		t.Errorf("versionString() = %q, want v1.2.3", got)
	}
}

// Unstamped ("dev") builds fall back to runtime/debug's module version; under
// `go test`, that's always the devel placeholder, so the function's own "dev"
// fallback is what must come out — proving the fallback chain doesn't panic
// or return an empty string when build info isn't a real release.
func TestVersionStringDevFallback(t *testing.T) {
	prev := version
	version = "dev"
	defer func() { version = prev }()

	if got := versionString(); got == "" {
		t.Error("versionString() should never return empty")
	}
}

// --version must print the version and signal "don't launch the TUI".
func TestRunVersionFlag(t *testing.T) {
	prev := version
	version = "v9.9.9"
	defer func() { version = prev }()

	var code int
	var launch bool
	out := smCaptureStdout(t, func() { code, launch = run([]string{"slack-tui", "--version"}) })

	if code != 0 || launch {
		t.Errorf("run(--version) = (%d, %v), want (0, false)", code, launch)
	}
	if !strings.Contains(out, "v9.9.9") {
		t.Errorf("expected the version printed, got %q", out)
	}
}

// The hidden --dump dev flag must render one frame of the mock workspace
// headlessly, at exactly the requested width, and never launch the TUI.
func TestRunDumpRendersMockFrame(t *testing.T) {
	smIsolateConfigDir(t)

	var code int
	var launch bool
	out := smCaptureStdout(t, func() {
		code, launch = run([]string{"slack-tui", "--dump", "100x30"})
	})
	if code != 0 || launch {
		t.Errorf("run(--dump) = (%d, %v), want (0, false)", code, launch)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output from --dump")
	}
	if w := lipgloss.Width(lines[0]); w != 100 {
		t.Errorf("first line width = %d, want 100", w)
	}
	// The mock workspace ("monospace-labs") must actually be what got
	// rendered — proves app.New() fell back to the mock, not an empty/erroring
	// live source (which would happen if a real token leaked into this test).
	if !strings.Contains(out, "monospace-labs") || !strings.Contains(out, "general") {
		t.Errorf("expected mock workspace content in the frame, got:\n%s", out)
	}
}

// A key replay after the size must reach the app before the frame is dumped
// — proves the comma-split replay loop actually feeds Update, not just parses.
func TestRunDumpReplaysKeys(t *testing.T) {
	smIsolateConfigDir(t)
	// "j" moves the sidebar cursor down one row; without replay, "engineering"
	// (the second channel) would not yet carry the cursor highlight this early.
	out := smCaptureStdout(t, func() {
		run([]string{"slack-tui", "--dump", "100x30", "j"})
	})
	if !strings.Contains(out, "engineering") {
		t.Errorf("expected the replayed key's effect to reach the dumped frame, got:\n%s", out)
	}
}

// A malformed --dump size must fall through to "launch the TUI" (today's
// real behavior) rather than erroring — main() is the only place that may
// act on launch=true, so this is safe to assert without starting Bubble Tea.
func TestRunDumpMalformedSizeFallsThroughToLaunch(t *testing.T) {
	code, launch := run([]string{"slack-tui", "--dump", "bogus"})
	if !launch {
		t.Error("a malformed --dump size should fall through to launch=true, matching current behavior")
	}
	if code != 0 {
		t.Errorf("code = %d, want 0 (unused when launch=true)", code)
	}
}

// --dump-ob renders an onboarding phase headlessly; wizard:theme must show
// the theme picker's cards.
func TestRunDumpObRendersOnboardingPhase(t *testing.T) {
	smIsolateConfigDir(t)
	var launch bool
	out := smCaptureStdout(t, func() {
		_, launch = run([]string{"slack-tui", "--dump-ob", "90x28", "wizard:theme"})
	})
	if launch {
		t.Error("--dump-ob should not fall through to launch")
	}
	for _, want := range []string{"Charcoal", "Midnight"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected theme card %q in the dumped onboarding frame, got:\n%s", want, out)
		}
	}
}

// --workspace <name> must set config.ActiveOverride for the rest of this
// process's run, and its two argv slots must be stripped before any later
// flag parsing sees them (dump still works with the shifted args).
func TestRunWorkspaceFlagSetsOverrideAndStripsArgs(t *testing.T) {
	smIsolateConfigDir(t)
	prevOverride := config.ActiveOverride
	t.Cleanup(func() { config.ActiveOverride = prevOverride })

	out := smCaptureStdout(t, func() {
		run([]string{"slack-tui", "--workspace", "acme", "--dump", "40x10"})
	})
	if config.ActiveOverride != "acme" {
		t.Errorf("config.ActiveOverride = %q, want acme", config.ActiveOverride)
	}
	if len(out) == 0 {
		t.Error("--dump after --workspace produced no output — args were not stripped correctly")
	}
}

// The "doctor" subcommand must actually dispatch to doctor.Run, not just
// parse the argument — exercised end to end with no tokens configured, which
// is a deterministic, hermetic exit (1, "no user token").
func TestRunDoctorSubcommand(t *testing.T) {
	smIsolateConfigDir(t)
	var code int
	out := smCaptureStdout(t, func() {
		code, _ = run([]string{"slack-tui", "doctor"})
	})
	if code != 1 {
		t.Errorf("run(doctor) with no token = code %d, want 1", code)
	}
	if !strings.Contains(out, "no user token") {
		t.Errorf("expected doctor's report in the output, got:\n%s", out)
	}
}

// FORCE_COLOR must force the truecolor profile; unset, run() must leave
// whatever profile was already active alone. Both directions are asserted
// against a profile this test set itself (not testenv.Pin's own forcing), so
// an inverted or deleted condition in run() would actually fail this.
func TestRunForceColorTogglesColorProfile(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	t.Run("set", func(t *testing.T) {
		t.Setenv("FORCE_COLOR", "1")
		lipgloss.SetColorProfile(termenv.Ascii)
		run([]string{"slack-tui"})
		if got := lipgloss.ColorProfile(); got != termenv.TrueColor {
			t.Errorf("profile = %v, want TrueColor with FORCE_COLOR set", got)
		}
	})

	t.Run("unset", func(t *testing.T) {
		t.Setenv("FORCE_COLOR", "")
		lipgloss.SetColorProfile(termenv.Ascii)
		run([]string{"slack-tui"})
		if got := lipgloss.ColorProfile(); got != termenv.Ascii {
			t.Errorf("profile = %v, want it left alone (Ascii) with FORCE_COLOR unset", got)
		}
	})
}

// A key replay after --dump-ob's target must actually reach the onboarding
// model before the frame is dumped — proves the loop reassigns m rather than
// discarding the result.
func TestRunDumpObReplaysKeys(t *testing.T) {
	smIsolateConfigDir(t)
	without := smCaptureStdout(t, func() {
		run([]string{"slack-tui", "--dump-ob", "90x28", "wizard:theme"})
	})
	withKey := smCaptureStdout(t, func() {
		run([]string{"slack-tui", "--dump-ob", "90x28", "wizard:theme", "j"})
	})
	if without == withKey {
		t.Error("replaying a selection-moving key ('j') should change the dumped frame, but it didn't")
	}
}

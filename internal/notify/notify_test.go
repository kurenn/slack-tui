package notify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Emoji are everywhere in chat, so truncation must cut runes: slicing bytes
// would leave a half-rune the daemon renders as a replacement character.
func TestTruncateIsRuneSafe(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "🎉"
	}
	got := truncate(long, 10)
	if r := []rune(got); len(r) != 10 {
		t.Errorf("len = %d runes, want 10", len(r))
	}
	for _, r := range got {
		if r == '�' {
			t.Fatal("truncation split a rune")
		}
	}
}

// A notification is one line; newlines from a multi-line message must collapse
// rather than reflowing the popup.
func TestTruncateCollapsesWhitespace(t *testing.T) {
	if got := truncate("hey\n\n  there\tyou", 100); got != "hey there you" {
		t.Errorf("got %q", got)
	}
	if got := truncate("short", 100); got != "short" {
		t.Errorf("unchanged text should pass through, got %q", got)
	}
}

// osascript takes a script rather than argv, so a message containing a quote
// would otherwise close the string and execute as AppleScript.
func TestQuoteEscapesAppleScript(t *testing.T) {
	for in, want := range map[string]string{
		`hi`:           `"hi"`,
		`say "hi"`:     `"say \"hi\""`,
		`back\slash`:   `"back\\slash"`,
		`" & do shell`: `"\" & do shell"`,
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

// Send must be harmless where no notifier exists — a tty, a container, CI.
func TestSendIsSafeWithoutANotifier(t *testing.T) {
	once.Do(func() {}) // pin detection without running it
	prevEnabled, prevFn := enabled, sendFn
	enabled, sendFn = false, nil
	defer func() { enabled, sendFn = prevEnabled, prevFn }()

	Send("#general", "should not panic")
}

// smResetNotify restores the package's detection seams after a test pins
// goos/lookPath, and forces detect() to run again on the next Available/Send
// call instead of reusing whatever a prior test (or TestMain's real machine
// probe) already cached in the sync.Once.
func smResetNotify(t *testing.T) {
	t.Helper()
	prevGoos, prevLookPath := goos, lookPath
	prevEnabled, prevSendFn := enabled, sendFn
	t.Cleanup(func() {
		goos, lookPath = prevGoos, prevLookPath
		once, enabled, sendFn = sync.Once{}, prevEnabled, prevSendFn
	})
	once = sync.Once{}
}

// smRecorderScript writes a POSIX shell script that appends each of its
// arguments as its own line to argv.log next to it, then returns the
// script's path. It stands in for the real notifier binary so detect()'s
// sendFn can be exercised without spawning notify-send/osascript for real.
func smRecorderScript(t *testing.T, dir string) string {
	t.Helper()
	log := filepath.Join(dir, "argv.log")
	script := filepath.Join(dir, "recorder.sh")
	body := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + log + "\"; done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// smWaitForArgv polls argv.log until it has at least wantLines lines (run()
// fires the recorder without waiting for it) and returns them, split. Fails
// the test on timeout rather than hanging the suite.
func smWaitForArgv(t *testing.T, dir string, wantLines int) []string {
	t.Helper()
	log := filepath.Join(dir, "argv.log")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(log)
		if err == nil {
			lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
			if len(lines) >= wantLines && lines[0] != "" {
				return lines
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("argv.log never reached %d lines", wantLines)
	return nil
}

// On Linux, detect() must pick notify-send, and Send must hand it the
// documented flags plus a truncated (not raw) body — this is the whole
// contract between the app and the freedesktop notifier.
func TestDetectLinuxUsesNotifySend(t *testing.T) {
	smResetNotify(t)
	goos = "linux"
	dir := t.TempDir()
	script := smRecorderScript(t, dir)
	lookPath = func(name string) (string, error) {
		if name == "notify-send" {
			return script, nil
		}
		return "", exec.ErrNotFound
	}

	if !Available() {
		t.Fatal("expected notify-send to be detected on linux")
	}
	longBody := strings.Repeat("z", 300) // well past bodyLimit (140 runes)
	Send("#general", longBody)

	argv := smWaitForArgv(t, dir, 4)
	if argv[0] != "--app-name=slack-tui" {
		t.Errorf("argv[0] = %q, want --app-name=slack-tui", argv[0])
	}
	if argv[1] != "--urgency=normal" {
		t.Errorf("argv[1] = %q, want --urgency=normal", argv[1])
	}
	if argv[2] != "#general" {
		t.Errorf("argv[2] (title) = %q, want #general", argv[2])
	}
	sentBody := argv[3]
	if r := []rune(sentBody); len(r) > 140 {
		t.Errorf("body sent to notify-send is %d runes, want <=140 (truncate-before-send regressed)", len(r))
	}
	if !strings.HasSuffix(sentBody, "…") {
		t.Errorf("truncated body %q should end with an ellipsis", sentBody)
	}
	if sentBody == longBody {
		t.Error("raw (untruncated) body was sent — truncate-before-send regressed")
	}
}

// On macOS with terminal-notifier installed, detect() must prefer it over
// osascript (a real app identity + clickable notification) and pass its
// specific flag set.
func TestDetectDarwinPrefersTerminalNotifier(t *testing.T) {
	smResetNotify(t)
	goos = "darwin"
	dir := t.TempDir()
	script := smRecorderScript(t, dir)
	lookPath = func(name string) (string, error) {
		if name == "terminal-notifier" {
			return script, nil
		}
		return "", exec.ErrNotFound // osascript would also resolve on a real Mac; must not be picked
	}

	if !Available() {
		t.Fatal("expected terminal-notifier to be detected on darwin")
	}
	Send("Ada", "hi there")

	argv := smWaitForArgv(t, dir, 5)
	want := []string{"-title", "slack-tui", "-subtitle", "Ada", "-message", "hi there"}
	if strings.Join(argv, "|") != strings.Join(want, "|") {
		t.Errorf("terminal-notifier argv = %v, want %v", argv, want)
	}
}

// Without terminal-notifier, detect() falls back to osascript, which takes a
// literal AppleScript string rather than argv — so the body must go through
// quote() at the call site, or a message containing a quote could break out
// of the string and run arbitrary AppleScript.
func TestDetectDarwinFallsBackToOsascript(t *testing.T) {
	smResetNotify(t)
	goos = "darwin"
	dir := t.TempDir()
	script := smRecorderScript(t, dir)
	lookPath = func(name string) (string, error) {
		if name == "osascript" {
			return script, nil
		}
		return "", exec.ErrNotFound
	}

	if !Available() {
		t.Fatal("expected osascript to be detected on darwin")
	}
	Send("Ada", `say "hi" & do shell script "rm -rf /"`)

	argv := smWaitForArgv(t, dir, 2)
	if argv[0] != "-e" {
		t.Fatalf("argv[0] = %q, want -e (osascript takes a script, not argv)", argv[0])
	}
	scriptArg := argv[1]
	if !strings.Contains(scriptArg, `\"hi\"`) {
		t.Errorf("quoted body not found in AppleScript arg: %q", scriptArg)
	}
	if strings.Contains(scriptArg, `" & do shell script "rm`) {
		t.Error("unescaped quote let the message break out of the AppleScript string literal")
	}
}

// With no known notifier on the platform (or an unsupported OS), Available
// must report false and Send must be a safe no-op — never a panic on a nil
// sendFn.
func TestDetectNoNotifierFound(t *testing.T) {
	smResetNotify(t)
	goos = "windows"
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	if Available() {
		t.Fatal("windows has no notifier wired up; Available() should be false")
	}
	Send("#general", "should not panic") // sendFn is nil; Send must not call it
}

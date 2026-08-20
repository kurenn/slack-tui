package notify

import "testing"

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

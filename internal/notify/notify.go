// Package notify posts desktop notifications through whatever the platform
// provides, and does nothing at all when it provides nothing — a plain tty, an
// ssh session or a container is a normal place to run this, not an error.
package notify

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// appName is what the notification daemon shows as the source. On Omarchy this
// is what groups slack-tui's notifications together in the shell.
const appName = "slack-tui"

// bodyLimit keeps a long message from becoming a wall of text on screen. The
// notification is a nudge; the message is in the TUI.
const bodyLimit = 140

var (
	once    sync.Once
	sendFn  func(title, body string)
	enabled bool
)

// Available reports whether this machine can show notifications, so callers can
// tell "turned off" apart from "not possible here".
func Available() bool {
	once.Do(detect)
	return enabled
}

// Send posts a notification. It is best-effort: failures are ignored, because a
// chat client must not fall over — or block — because a notification daemon
// isn't running.
func Send(title, body string) {
	once.Do(detect)
	if !enabled {
		return
	}
	sendFn(title, truncate(body, bodyLimit))
}

func detect() {
	switch runtime.GOOS {
	case "darwin":
		// terminal-notifier gives a real app identity and a clickable
		// notification; osascript is the fallback that's always present.
		if path, err := exec.LookPath("terminal-notifier"); err == nil {
			enabled, sendFn = true, func(title, body string) {
				run(path, "-title", appName, "-subtitle", title, "-message", body)
			}
			return
		}
		if path, err := exec.LookPath("osascript"); err == nil {
			enabled, sendFn = true, func(title, body string) {
				// osascript takes a script, not argv, so both fields are quoted
				// — a message containing a quote would otherwise end the string
				// and run as AppleScript.
				run(path, "-e", "display notification "+quote(body)+
					" with title "+quote(appName)+" subtitle "+quote(title))
			}
			return
		}
	case "windows":
		// No dependency-free toast worth the complexity; the terminal bell and
		// the window title still carry the alert.
	default: // linux, bsd — the freedesktop spec
		if path, err := exec.LookPath("notify-send"); err == nil {
			enabled, sendFn = true, func(title, body string) {
				run(path, "--app-name="+appName, "--urgency=normal", title, body)
			}
			return
		}
	}
}

// run fires the notifier without waiting for it. Nothing downstream cares about
// the result, and a hung daemon must not stall the caller.
func run(path string, args ...string) {
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return
	}
	go func() { _ = cmd.Wait() }() // reap it; a zombie per message adds up
}

// quote escapes a string for embedding in an AppleScript literal.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// truncate shortens to n runes, not bytes — cutting mid-rune would render as a
// replacement character, and chat is full of emoji.
func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ") // notifications are single-line
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

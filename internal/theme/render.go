package theme

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

// FillBg truncates/pads s to width and paints bg continuously behind it. It
// re-asserts the background after every SGR reset embedded in s — lipgloss emits
// a reset at the end of each styled span, which would otherwise clear the
// background mid-line and leave ragged, partial bars behind colored text.
func FillBg(s string, width int, bg lipgloss.Color) string {
	s = ansi.Truncate(s, width, "")
	seq := bgSeq(bg)
	pad := width - lipgloss.Width(s)
	if seq == "" {
		if pad > 0 {
			return s + strings.Repeat(" ", pad)
		}
		return s
	}
	out := seq + strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+seq)
	if pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out + "\x1b[0m"
}

func bgSeq(c lipgloss.Color) string {
	col, err := colorful.Hex(string(c))
	if err != nil {
		return ""
	}
	r, g, b := col.RGB255()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

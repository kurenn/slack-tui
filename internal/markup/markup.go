// Package markup ports the handoff's renderText syntax tokenizer: it splits text
// on fenced ``` blocks and inline-tokenizes `code`, @mentions, #channels and urls,
// styling each with the palette's distinct hues. Output is a lipgloss-styled
// string ready to drop into a message body.
package markup

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/abrahamkuri/slack-tui/internal/theme"
)

// tokRe mirrors the prototype's TOK_RE: `code`, @mention, #channel, url.
var tokRe = regexp.MustCompile("(`[^`]+`)|(@[A-Za-z0-9_.]+)|(#[A-Za-z0-9_-]+)|(https?://[^\\s]+)")

// Inline styles a single line, tokenizing inline spans. @you is highlighted as a
// self-mention. width is the wrap width; <=0 disables wrapping.
func Inline(p theme.Palette, line string) string {
	code := lipgloss.NewStyle().Foreground(p.Orange).Background(p.CodeBg)
	mention := lipgloss.NewStyle().Foreground(p.Blue)
	mentionMe := lipgloss.NewStyle().Foreground(p.Orange).Bold(true)
	channel := lipgloss.NewStyle().Foreground(p.Blue)
	url := lipgloss.NewStyle().Foreground(p.Cyan).Underline(true)

	var b strings.Builder
	last := 0
	for _, m := range tokRe.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > last {
			b.WriteString(line[last:m[0]])
		}
		tok := line[m[0]:m[1]]
		switch {
		case m[2] >= 0: // `code`
			b.WriteString(code.Render(tok[1 : len(tok)-1]))
		case m[4] >= 0: // @mention
			if strings.EqualFold(tok, "@you") {
				b.WriteString(mentionMe.Render(tok))
			} else {
				b.WriteString(mention.Render(tok))
			}
		case m[6] >= 0: // #channel
			b.WriteString(channel.Render(tok))
		case m[8] >= 0: // url
			b.WriteString(url.Render(tok))
		}
		last = m[1]
	}
	if last < len(line) {
		b.WriteString(line[last:])
	}
	return b.String()
}

// Render styles a full message body: fenced ``` blocks become code blocks, other
// segments are line-tokenized. Returns a multi-line styled string.
func Render(p theme.Palette, text string) string {
	codeBlock := lipgloss.NewStyle().Foreground(p.Fg).Background(p.CodeBg).Padding(0, 1)

	parts := strings.Split(text, "```")
	var out []string
	for i, seg := range parts {
		if i%2 == 1 { // fenced block
			seg = strings.TrimPrefix(seg, "\n")
			seg = strings.TrimSuffix(seg, "\n")
			out = append(out, codeBlock.Render(seg))
			continue
		}
		if seg == "" {
			continue
		}
		for _, ln := range strings.Split(seg, "\n") {
			out = append(out, Inline(p, ln))
		}
	}
	return strings.Join(out, "\n")
}

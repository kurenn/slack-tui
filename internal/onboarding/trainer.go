package onboarding

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// drill describes one keyboard drill in the trainer.
type drill struct {
	id, label, keys string
}

var drills = []drill{
	{"nav", "navigate", "j / k"},
	{"write", "compose", "i  ↵"},
	{"thread", "threads", "t"},
	{"palette", "palette", "⌃K"},
}

// miniMsg is a row in the trainer's miniature message list.
type miniMsg struct {
	user, color, time, text, react string
	replies                        int
	mentionMe                      bool
}

var miniMsgs = []miniMsg{
	{user: "ada.k", color: "purple", time: "09:21", text: "pushed the render-loop refactor — only repaints dirty cells now", react: "🔥 6", replies: 3},
	{user: "lin.z", color: "green", time: "09:27", text: "that is going to make scroll-back buttery. nice work"},
	{user: "lin.z", color: "green", time: "09:38", text: "hey @you — can you take the keyboard-focus ticket?", mentionMe: true},
}

// trainerState holds the interactive trainer's mini-app state.
type trainerState struct {
	drill       int
	done        [4]bool
	sel         int
	movedDown   bool
	movedUp     bool
	mode        string // normal | insert
	mini        textinput.Model
	sent        bool
	threadOpen  bool
	paletteOpen bool
}

func newTrainer() trainerState {
	ti := textinput.New()
	ti.Prompt = ""
	return trainerState{mode: "normal", mini: ti, sel: 0}
}

// needsEsc reports whether the trainer must consume Esc for the current drill
// (canceling the compose INSERT, or closing the practice palette). Otherwise Esc
// is free to navigate the wizard back a step.
func (t trainerState) needsEsc() bool {
	return (t.mode == "insert" && t.drill == 1) || t.paletteOpen
}

// markDone marks a drill complete and returns the auto-advance command.
func (m *Model) markDone(idx int) tea.Cmd {
	if m.trainer.done[idx] {
		return nil
	}
	m.trainer.done[idx] = true
	return tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return advanceDrillMsg{} })
}

// advanceDrill moves to the next drill (or completes the tutorial).
func (m Model) advanceDrill() (Model, tea.Cmd) {
	t := &m.trainer
	t.drill++
	if t.drill >= len(drills) {
		t.drill = len(drills) - 1
		m.kbDone = true
	}
	t.mode = "normal"
	t.threadOpen = false
	t.paletteOpen = false
	t.mini.Blur()
	return m, nil
}

// trainerKey routes a key into the trainer mini-app while the keyboard step is active.
func (m Model) trainerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := msg.String()
	t := &m.trainer

	// INSERT mode (compose drill): input owns typing.
	if t.mode == "insert" && t.drill == 1 {
		switch k {
		case "esc":
			t.mode = "normal"
			t.mini.Blur()
			return m, nil
		case "enter":
			if t.mini.Value() != "" {
				t.sent = true
				t.mini.SetValue("")
				t.mode = "normal"
				t.mini.Blur()
				return m, m.markDone(1)
			}
			return m, nil
		}
		var cmd tea.Cmd
		t.mini, cmd = t.mini.Update(msg)
		return m, cmd
	}

	if t.paletteOpen {
		if k == "esc" {
			t.paletteOpen = false
			return m, m.markDone(3)
		}
		return m, nil
	}

	if k == "ctrl+k" && t.drill == 3 {
		t.paletteOpen = true
		return m, nil
	}

	switch t.drill {
	case 0: // navigate
		switch k {
		case "j", "down":
			t.sel = clamp(t.sel+1, 0, len(miniMsgs)-1)
			t.movedDown = true
		case "k", "up":
			t.sel = clamp(t.sel-1, 0, len(miniMsgs)-1)
			t.movedUp = true
		}
		if t.movedDown && t.movedUp {
			return m, m.markDone(0)
		}
	case 1: // compose
		if k == "i" {
			t.mode = "insert"
			return m, t.mini.Focus()
		}
	case 2: // threads
		switch k {
		case "t", "enter":
			t.threadOpen = true
			return m, m.markDone(2)
		case "esc":
			t.threadOpen = false
		}
	}
	return m, nil
}

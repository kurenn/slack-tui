# slack-tui

A keyboard-first, vim-modal Slack client that runs **inside your terminal**
(Ghostty et al.) — like `vim` or `lazygit`, not a separate app. Built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

> Status: **early scaffold.** The theme system, mock data layer, and the box-drawing
> Pane primitive are in place and render a static three-pane shell. The keyboard
> engine, command palette, threads, onboarding, and real Slack integration are
> being built in sequence (see _Roadmap_).

## Run

```sh
go run .           # launch the TUI (currently the static shell)
go run . --dump 100x30   # render one frame to stdout (headless / screenshots)
```

Press `T` to cycle themes, `q` / `Ctrl-C` to quit.

## Design source

Recreated from the high-fidelity handoff in `design_handoff_slack_tui/`. The
prototype is a React/HTML "fake terminal"; this is the real thing. Notable
terminal-vs-web deltas: the app can't change the host terminal's **font** (that's
Ghostty's config) and `Cmd-K` becomes `Ctrl-K` (terminals never see ⌘).

## Architecture

```
main.go                 entry; alt-screen; loads prefs → (onboarding | app)
internal/
  theme/    5 palettes + 7 accents + density → resolved lipgloss colors
  data/     domain types + the mock monospace-labs workspace (Datasource shape)
  markup/   syntax tokenizer: `code` @mention #channel url + ```fences```
  config/   prefs.json at ~/.config/slack-tui (the onboarding→app handoff seam)
  ui/pane/  the box-drawing pane primitive (title embedded in the top rule)
```

## Roadmap

1. ✅ Foundations — theme tokens, mock data, config, Pane primitive, static shell
2. ✅ Modal keyboard engine (NORMAL/INSERT, focus routing)
3. ✅ Threads + composer send + scroll-into-view
4. ⬜ Command palette
5. ⬜ Onboarding (boot → auth → wizard → keyboard trainer → launch)
6. ⬜ Prefs handoff + live theme/accent/density switching
7. ⬜ Real Slack data source (Web API reads + Socket Mode/poll for live)

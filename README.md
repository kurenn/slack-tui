# slack-tui

A keyboard-first, vim-modal Slack client that runs **inside your terminal**
(Ghostty et al.) — like `vim` or `lazygit`, not a separate app. Built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

> Status: **working client.** Three-pane shell, vim-modal keyboard engine, command
> palette, threads, full onboarding, and real Slack (Web API reads/sends, presence,
> Socket Mode live unread). Falls back to a mock workspace when no token is set.

## Run

```sh
go run .                 # launch (mock data if no token; real Slack if a token is set)
go run . --dump 100x30   # render one frame to stdout (headless / screenshots)
```

### Connecting to Slack

Create an app from `slack-app-manifest.yaml`, then provide a **user token** (`xoxp-…`)
— either paste it in the onboarding "Paste an auth token" screen (persisted to
`~/.config/slack-tui/tokens.json`, 0600) or via env var. Env vars override the file:

```sh
SLACK_USER_TOKEN=xoxp-…                          # required for real Slack
SLACK_APP_TOKEN=xapp-… SLACK_BOT_TOKEN=xoxb-…    # optional: Socket Mode live channel unread
```

Keys: `j/k`/`gg`/`G` move · `Tab`/`h`/`l` panes · `t`/`Enter` thread · `i` write ·
`Ctrl-K` palette · `,` settings · `Ctrl-R` refresh · `q` quit.

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
4. ✅ Command palette (`Ctrl-K`)
5. ✅ Onboarding (boot → auth → wizard → keyboard trainer → launch)
6. ✅ Prefs hand-off (onboarding writes `prefs.json`, app adopts theme/accent/density/status/handle)
7. ⬜ Real Slack data source (Web API reads + Socket Mode/poll for live)

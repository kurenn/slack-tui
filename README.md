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

Create an app from `slack-app-manifest.yaml`, then connect one of two ways:

**Browser sign-in (recommended).** Copy the app's **Client ID / Secret** (Basic
Information) into env or `~/.config/slack-tui/oauth.json`, then:

```sh
SLACK_CLIENT_ID=… SLACK_CLIENT_SECRET=… slack-tui login   # opens the browser, saves tokens
```

…or pick **"Sign in with Slack"** in onboarding. Tokens are saved to
`~/.config/slack-tui/tokens.json` (0600).

**Manual token.** Paste a **user token** (`xoxp-…`) in the onboarding token screen,
or set env vars (env overrides the saved file, per-token):

```sh
SLACK_USER_TOKEN=xoxp-…                          # required for real Slack
SLACK_APP_TOKEN=xapp-… SLACK_BOT_TOKEN=xoxb-…    # optional: Socket Mode live channel unread
```

The **app-level token** (`xapp-…`) for Socket Mode is never issued by OAuth — set
it via `SLACK_APP_TOKEN` or the onboarding token form.

Keys: `j/k`/`gg`/`G` move · `Ctrl-d`/`Ctrl-u` scroll · `Tab`/`h`/`l` panes ·
`t`/`Enter` thread · `i` write · `]`/`[` next/prev unread · `Ctrl-K` palette ·
`,` settings · `Ctrl-R` refresh · `q` quit.

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

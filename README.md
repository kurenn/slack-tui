<div align="center">

# slack-tui

**A keyboard-first, vim-modal Slack client for your terminal.**

Like `vim` or `lazygit` — not another Electron window.
Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

[![ci](https://github.com/kurenn/slack-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/kurenn/slack-tui/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/kurenn/slack-tui)](https://github.com/kurenn/slack-tui/releases)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

<img src="docs/main.png" width="760" alt="slack-tui main view" />

</div>

## Features

- **Vim-modal** — NORMAL/INSERT modes, `j/k`, `gg/G`, `Ctrl-d/u`, `/` find, `dd` delete
- **Everything async** — sends are optimistic, channels load in the background, the UI never freezes
- **Threads, reactions, edits** — open threads, react with any emoji, edit and delete your messages
- **Autocomplete** — `@` pops handles (mentions actually ping), `:` pops emoji
- **Fuzzy everything** — command palette (`Ctrl-K`), workspace search (`s`), channel browser & join
- **Lives in the background** — Socket Mode live unread (self-healing), terminal bell on mentions, unread count in the terminal title, `── new ──` divider at first unread
- **5 themes × 7 accents**, group DMs, per-conversation drafts, multi-line composer
- **Mock workspace built in** — run it with zero setup to try the feel

<div align="center">
<img src="docs/palette.png" width="420" alt="command palette" /> <img src="docs/autocomplete.png" width="420" alt="mention autocomplete" />
<img src="docs/thread.png" width="760" alt="thread view" />
</div>

## Install

```sh
go install github.com/kurenn/slack-tui@latest
```

…or grab a binary from the [releases page](https://github.com/kurenn/slack-tui/releases).

```sh
slack-tui          # no token? you get a mock workspace to play with
```

## Connect to Slack

1. Create a Slack app from [`slack-app-manifest.yaml`](slack-app-manifest.yaml)
   (api.slack.com/apps → *Create New App* → *From an app manifest*).
2. Sign in with your browser:

```sh
SLACK_CLIENT_ID=… SLACK_CLIENT_SECRET=… slack-tui login
```

…or pick **"Sign in with Slack"** in onboarding, or paste a user token
(`xoxp-…`) directly. Tokens are stored in `~/.config/slack-tui/tokens.json`
(0600). Env vars (`SLACK_USER_TOKEN`, `SLACK_APP_TOKEN`, `SLACK_BOT_TOKEN`)
override per-token.

For **live** channel unread, also generate an app-level token (`xapp-…`,
Socket Mode) and invite the bot to the channels you care about — without it,
unread badges refresh on a slow poll instead.

## Keys

| | |
|---|---|
| `j/k` `gg/G` `Ctrl-d/u` | move · jump · half-page |
| `Tab` `h/l` | switch panes |
| `Enter` / `t` | open thread |
| `i` · `r` | write · reply in thread |
| `Alt-Enter` | newline in the composer |
| `@…` `:…` | autocomplete mentions / emoji (`Tab` accepts) |
| `a` · `e` · `dd` · `y` | react · edit · delete · yank |
| `o` | open message links/files |
| `/` `n/N` · `s` | find in channel · search workspace |
| `]` `[` | next/prev unread |
| `Ctrl-K` | command palette (fuzzy) |
| `,` · `?` | settings · help |
| `Esc` · `q` | close/dismiss · quit |

The mouse wheel scrolls. `?` shows the full keymap in-app.

## Development

```sh
go run .                          # mock workspace, no setup
go run . --dump 100x30            # render one frame to stdout (headless)
go run . --dump 100x30 "ctrl+k"   # …after replaying keys
go test ./...                     # hermetic — never touches the network
```

The architecture in one breath: `internal/source` abstracts the backend (a
mock and a real Slack client behind one interface — network calls run inside
`tea.Cmd`s, never on the UI thread), `internal/app` is the Bubble Tea model
with the modal keyboard engine, and `internal/ui` renders the panes.

## License

[MIT](LICENSE)

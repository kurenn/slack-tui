# Contributing

Thanks for your interest! A few things to know before opening a PR.

## The prime directive

slack-tui stays **super lightweight, fast, and simple — vim-oriented**. It's
positioned like `vim` or `lazygit`, not a feature-complete Slack replacement.
Concretely, that means:

- **No new dependencies** without a very good reason. Generated data files
  (like the emoji map) beat libraries.
- **Vim idioms**: modal keys, quickfix-style lists, small composable actions.
  If a feature can be a keybinding plus one API call, that's the right shape.
- Features that need background services, config systems, graphics protocols,
  or hosted infrastructure are almost certainly out of scope — open an issue
  to discuss before writing code.

## Development

```sh
go run .                          # mock workspace, zero setup
go run . --dump 100x30            # render one frame headlessly
go run . --dump 100x30 "ctrl+k"   # …after replaying keys
go vet ./... && go test ./...     # must be clean before any PR
```

Tests are **hermetic** — they run against the in-memory mock
(`source.NewMock()` via the `newTest()` helper) and must never touch the
network or the user's config files. Network calls in the app itself live
inside `tea.Cmd`s, never on the UI thread.

## Architecture in one breath

`internal/source` abstracts the backend (mock + real Slack behind one
interface) · `internal/app` is the Bubble Tea model with the modal keyboard
engine · `internal/ui` renders the panes · `internal/config` owns
`~/.config/slack-tui` · `internal/doctor` is the diagnostic subcommand.

## PRs

- Keep diffs tight and single-purpose; add a test for behavior changes.
- Match the existing style: comments explain *why*, not what.
- If you're touching Slack interop, `slack-tui doctor` output and a note on
  which token type you tested with helps review a lot.

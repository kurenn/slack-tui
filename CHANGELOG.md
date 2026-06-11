# Changelog

slack-tui follows semver-ish tags; every release ships binaries for
macOS/Linux/Windows plus a Homebrew formula via goreleaser.

## v0.3.0

- **Workspace switching** — `slack-tui login` now saves each workspace under
  its team name; switch in-app via `Ctrl-K` → *Switch workspace* (one live
  workspace at a time, tmux-session style) or launch with
  `slack-tui --workspace <name>`. Legacy single-workspace `tokens.json`
  migrates automatically.
- Socket Mode connections now tear down cleanly on switch
- `doctor` lists signed-in workspaces

## v0.2.1

- Full emoji coverage: ~2,000 Slack short names render as glyphs (generated
  from iamcal/emoji-data, the dataset Slack itself uses) in reaction pills,
  the reaction picker, and `:` autocomplete
- Command palette ranks conversations most-recently-opened first
- Drafts and palette recency survive restarts (`state.json`, saved on quit)

## v0.2.0

- **Threads inbox** (`T`) — filterable list of every thread you're in, with
  new-reply badges; Enter jumps to the conversation and opens the thread
- **`slack-tui doctor`** — diagnoses config locations, token sources (env vs
  file, and which wins), auth, granted-vs-required scopes, and Socket Mode
- **Live active conversation** — socket events apply instantly: new messages
  append in real time, thread replies land in the open pane, and bot/agent
  replies (`bot_message` events) are no longer dropped

## v0.1.x

- v0.1.7 — config standardized on `~/.config/slack-tui` (XDG), legacy
  location still read
- v0.1.6 — fix invalid user scope (`channels:join` → `channels:write`)
- v0.1.5 — thread replies surfaced: inline preview of the latest reply,
  auto-fetch + terminal bell when someone answers a thread you started
- v0.1.4 — Block Kit messages render (sections, context, rich text, buttons
  as placeholders); bot authors resolve; restart-proof composer
- v0.1.3 — composer paste fixes (soft-wrap growth, cursor visibility);
  arrow keys mirror hjkl
- v0.1.2 — install script + Homebrew tap (formula)
- v0.1.0 — first release: vim-modal three-pane client, threads, reactions,
  edit/delete, search, autocomplete, group DMs, live unread via Socket Mode

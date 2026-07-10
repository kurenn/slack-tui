# Changelog

slack-tui follows semver-ish tags; every release ships binaries for
macOS/Linux/Windows plus a Homebrew formula via goreleaser.

## v0.5.2

- **Unread that keeps up with a big DM list** — the DM unread poll used to fire
  one `conversations.history` call per DM for *every* DM each round; with a large
  DM list (100s) that instantly exceeds Slack's per-app rate limit, so the round
  aborts and most counts never refresh (and the flood throttles everything else).
  It now polls a bounded, recency-prioritized, rotating subset: your most
  recently-used DMs every round, plus a rotating window of the dormant tail.
- **Read state sticks** — a slow, rate-limited unread poll could return a count
  computed *before* you opened a conversation and re-flag it as unread. Poll
  results for a conversation read since the poll fired are now ignored.
- **Channel badges stop ballooning** — for Socket Mode users, channel unread is
  driven by live events, which were counting *thread replies* too, inflating
  thread-heavy channels far past Slack's own count. Only main-timeline messages
  bump the badge now; a thread reply that `@`-mentions you still counts.

## v0.5.1

- **Emoji shortcodes** — `:warning:`, `:tada:`, etc. in message text now render
  as glyphs (⚠️ 🎉) via the existing emoji table; unknown/custom names stay literal.
- **Slash commands** — type a command in the composer: `/shrug` `/me` `/away`
  `/active` `/dnd [min]` `/status <text>` `/dm @user` `/leave` `/search <q>`.
  Each maps to a real Slack API; unknown commands flash "not supported" (Slack's
  generic command API isn't open to OAuth apps), and a `/path` still sends as text.
- **Code rendering** — inline `` `code` `` and ```` ``` ```` blocks render styled,
  now including app/bot Block-Kit (`rich_text`) messages that previously dropped
  their code formatting.

## v0.5.0

- **Unread, for real** — per-conversation unread is now derived from the
  server-side `last_read` marker vs. recent history (`conversations.info`'s
  `unread_count_display` is dead for OAuth tokens), so the sidebar finally shows
  accurate unread. Unread rows get a brighter filled dot (orange for mentions).
- **Hide channels & DMs** — `x` hides a noisy conversation from the sidebar
  (local, reversible; still findable via search/`Ctrl-K`). It auto-resurfaces on
  any new message; un-hide from the palette.
- **Mouse text-selection** — drag to select text in the message pane; the exact
  substring is copied to the clipboard on release, with a `✓ copied` toast.
- **Attach/upload files** — drag a file onto the message pane (or press `A` to
  type a path) to stage it, then send it to the active conversation with an
  optional comment. Multiple files post as one message. (Needs `files:write`.)
- **Download files** — `S` saves the selected message's file(s) to `~/Downloads`
  (collision- and symlink-safe). (Needs `files:read`.)
- **Thread pane** — bigger by default, **mouse-draggable divider** to resize
  (persisted), mouse text-selection inside threads, and `y` to yank a reply.

## v0.4.0

- **Real presence** — DM partners' online/away dots are now live, refreshed
  on a slow (60s) poll via `users.getPresence` (bounded + rate-limit-guarded);
  no longer hardcoded to "online"
- **Faster message pane** — the per-keystroke geometry recompute is now cached
  (keyed by conversation/width/density/content generation), halving the
  render work on every j/k as history grows
- Read calls (history, thread replies) are now time-bounded, so a dropped
  connection surfaces an error instead of an eternal "loading…"

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

# Changelog

slack-tui follows semver-ish tags; every release ships binaries for
macOS/Linux/Windows plus a Homebrew formula via goreleaser.

## v0.6.1

- **Notifications actually fire** — every alert path, the terminal bell included,
  hung off a Socket Mode event. Socket Mode needs the `xapp` and `xoxb` tokens,
  and since v0.6.0 those can't be issued by a browser sign-in at all, so the
  default install could never ring or notify for anything. The unread poll now
  raises the alert itself: DMs on any increase, channels only when the new
  messages actually mention you (which also lights the mention dot that polling
  never set).
- **The unread poll was over Slack's rate limit** — measured against a real
  workspace it spent ~51 `conversations.history` calls/minute against a Tier-3
  ceiling of ~50, so rounds were aborting mid-sweep on 429s and leaving counts
  stale. Channels polled every channel every round with no bound at all — the
  same failure v0.5.2 fixed for DMs, never applied to channels. Channels are now
  bounded the same way, and the budget goes where it changes what you see: DM
  unread refreshes every 25s instead of 45s, on ~15% less total load. A test does
  the arithmetic, so changing an interval without re-checking the budget fails
  CI instead of producing 429s.
- **`slack-tui login` no longer cries wolf** — it told you a rotating token
  couldn't be refreshed, which stopped being true two commits after the warning
  was written.
- **Onboarding doesn't ask you to sign in twice** — `slack-tui setup` saves
  tokens without touching prefs, so the next launch put you back on the auth
  screen minutes after you'd authenticated.
- **`doctor` stops inventing a legacy config dir** — on any Linux box with
  `XDG_CONFIG_HOME` set it compared the config directory against itself and
  always warned.
- Test coverage raised from 45.6% to 84.1%, with CI now enforcing a floor and
  asserting that the suite doesn't touch the machine running it.

## v0.6.0

- **Sign in from the browser, without a client secret** — `slack-tui setup`
  creates your Slack app and signs you in: manifest to your clipboard, browser
  opened, one prompt for the Client ID, done. Onboarding does the same three
  steps in place, so picking "Sign in with Slack" no longer dead-ends on the
  paste-a-token screen. Sign-in is PKCE, which Slack in fact *requires* for a
  loopback redirect — so there is no client secret to handle anywhere, and any
  left in `oauth.json` is ignored and can be deleted.
- **Tokens refresh themselves** — Slack forces token rotation for a loopback
  redirect regardless of the app's rotation setting, so the access token now
  expires in ~12 hours. slack-tui renews it within 30 minutes of expiry, at
  launch and on its poll loop, and `doctor` shows the time remaining. The
  replacement is written to disk before it's used, so an interrupted refresh
  costs nothing worse than one `slack-tui login`.
- **Sign-in survives a busy port** — the callback used a single hardcoded 9899
  and failed outright when anything held it, most often a previous sign-in whose
  socket hadn't been released. It now takes the first free port in 9899–9903;
  all five are registered in the manifest, and a test fails if code and manifest
  drift.
- **Follows your desktop theme on [Omarchy](https://omarchy.org)** — the palette
  comes from the active theme and repaints when you run `omarchy theme set`, no
  restart, light themes included. Onboarding skips the colour questions there,
  since the desktop already answers them. Settings still pins a fixed palette,
  and nothing changes off Omarchy.
- **Onboarding tells the truth** — a real sign-in was greeted by the mock's name
  ("workspace @monospace-labs") in three places; it now shows the workspace you
  actually authorized. The "Single sign-on (SSO)" option is gone: it animated a
  fake sign-in and dropped you in the demo workspace, which is fine in a
  screenshot and misleading in a tool you install.
- **Bot tokens for Socket Mode are now copied by hand** — Slack refuses bot
  scopes on a loopback redirect, from any app, so the `xoxb-…` token joins the
  `xapp-…` one as something you paste from your app's admin page. Live unread is
  unchanged once both are set.
- **Desktop notifications** — mentions, DMs and replies to your own threads now
  raise a real notification (`notify-send` on Linux, `terminal-notifier` or
  `osascript` on macOS) alongside the existing terminal bell, using the same
  rule for what's worth interrupting for. Toggle in settings (`,`); a machine
  with no notifier reports "Unavailable" rather than looking switched off.
- **Setup instructions for agents** — [`docs/agent-setup.md`](docs/agent-setup.md)
  walks a coding agent through installing and connecting slack-tui, and is
  explicit about the two steps that need a human.
- Fixed: `go test ./...` overwrote the real `~/.config/slack-tui` on any machine
  with `XDG_CONFIG_HOME` set — which is most Linux desktops. Tests isolate both
  that and the desktop-theme lookup now.

## v0.5.3

- **Read state now actually reaches Slack** — `conversations.mark` needs a
  `*:write` scope per conversation kind, and only `channels:write` was ever
  requested. Marks succeeded for public channels and failed with
  `missing_scope` for DMs, group DMs and private channels — silently, since the
  result was discarded and the sidebar badge clears locally either way. The
  symptom: read everything in the TUI, then find it all still bold in Slack
  web/desktop. `groups:write`, `im:write` and `mpim:write` are now requested;
  **existing installs must re-authorize** (update the app's scopes from the
  manifest, then `slack-tui login` again) — `slack-tui doctor` names the
  missing scopes and the feature each one breaks.
- **A mark-read failure is visible** — the first rejected mark now surfaces in
  the error banner ("read state not syncing to Slack…") instead of vanishing.
  It reports once, not on every poll.
- **Sending marks the conversation read** — posting via the API doesn't advance
  your own read marker the way the official clients do, so a conversation you
  just sent in stayed unread everywhere else until the next poll (or forever, if
  you switched away or quit). The send's ack marks it now.
- **Optimistic sends no longer break the mark** — a mark firing between a send
  and its ack passed the local `pending-N` ID as a timestamp, which Slack
  rejects. It falls back to the newest acked message.

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

# Test-coverage plan: 45.6% → 80%

Audience: implementing agents working **in parallel**. Each work unit is self-contained,
owns an explicit set of files, and never edits another unit's files. Read the whole
"Ground rules" and "Anti-patterns" sections before writing a single test.

## 0. Ground truth and how to measure

Go is not on `PATH`. Every shell that builds or tests must first run:

```sh
export PATH="/tmp/claude-1000/-home-kurito-workspace-slack-tui/b0ddf67d-be26-459c-867c-a943bcec5cd5/scratchpad/go/bin:$PATH"
```

The metric is the statement-weighted total across **all** packages (untested packages
`internal/data` and `internal/root` count at 0), measured with own-package tests only:

```sh
go test ./... -coverprofile=cover.out -covermode=count
go tool cover -func=cover.out | tail -1     # baseline prints: total: (statements) 45.6%
```

Acceptance gate: that `total` line ≥ 80.0%, with `go vet ./... && go test ./...` clean.
Do **not** use `-coverpkg=./...` — cross-package coverage inflates the number (it reads
50.9% today) and is not the metric.

## 1. Per-package targets and the arithmetic

Statement counts measured from the merged profile (they are the exact weights):

| Package                | Stmts | Now          | Target | Covered @ target | Gain |
|------------------------|------:|-------------:|-------:|-----------------:|-----:|
| main (repo root)       |   131 |  0.0% (0)    |   50%  |               66 |  +66 |
| internal/app           |  2407 | 63.5% (1528) |   85%  |             2046 | +518 |
| internal/auth          |   113 | 40.7% (46)   |   85%  |               96 |  +50 |
| internal/config        |   177 | 57.1% (101)  |   90%  |              159 |  +58 |
| internal/data          |    13 |  0.0% (0)    |  100%  |               13 |  +13 |
| internal/doctor        |   156 | 28.2% (44)   |   85%  |              133 |  +89 |
| internal/markup        |    38 | 94.7% (36)   |   keep |               36 |    0 |
| internal/notify        |    30 | 30.0% (9)    |   80%  |               24 |  +15 |
| internal/onboarding    |  1047 | 31.1% (326)  |   78%  |              817 | +491 |
| internal/root          |    35 |  0.0% (0)    |   75%  |               26 |  +26 |
| internal/source        |   649 | 22.8% (148)  |   78%  |              506 | +358 |
| internal/theme         |   105 | 51.4% (54)   |   90%  |               95 |  +41 |
| internal/ui/components |   309 | 19.4% (60)   |   90%  |              278 | +218 |
| internal/ui/pane       |    60 | 90.0% (54)   |   93%  |               56 |   +2 |
| **Total**              | **5270** | **45.6% (2406)** |  | **4351**     |      |

4351 / 5270 = **82.6%**, i.e. ~135 statements (≈2.6 points) of buffer above the
4216-statement 80% line. The buffer exists because some targets will land a few points
short (app 85% and onboarding 78% are the ones most likely to slip). If a unit finds a
listed test would be theater (see §3), it drops the test and reports the shortfall
rather than writing it — the buffer is there to absorb exactly that.

The three heavyweights are app (2407 stmts), onboarding (1047) and source (649) —
together 78% of the repo. **No plan that skips any of them can reach 80%**; conversely,
the tiny packages (data, root, notify, pane) matter only because they are nearly free.

Per-file uncovered-statement map (own-package tests; the shopping list):

```
app:        app.go 264 · live.go 129 · view.go 75 · msgactions.go 61 · attach.go 43
            picker.go 43 · suggest.go 43 · statustext.go 39 · slash.go 35 · settings.go 34
            help.go 24 · find.go 22 · palette.go 21 · confirm.go 19 · select.go 17 · dump.go 7
onboarding: view.go 191 · wizard_view.go 170 · trainer_view.go 148 · keys.go 74
            typewriter.go 56 · onboarding.go 50 · dump.go 28
source:     slack.go 378 · mock.go 83 · socket.go 40
others:     doctor.go 112 · main.go 76 · setup.go 55 · auth/oauth.go 67 · config.go 49
            components: palette.go 55 · sidebar.go 60 · messages.go 52 · composer.go 26
            statusbar.go 19 · thread.go 16 · style.go 12 · titlebar.go 9
            theme: theme.go 27 · render.go 16 · omarchy.go 8 · root.go 35 · state.go 9
```

## 2. Ground rules (from CONTRIBUTING.md — non-negotiable)

- **Hermetic**: no network, ever. `httptest.Server` / loopback listeners are fine (they
  never leave the machine); `slack.com` is not. Never read or write the user's real
  config: every test that can touch config calls `isolateConfigDir(t)` (sets
  `XDG_CONFIG_HOME` to a temp dir — the helper already exists in
  `internal/app/app_test.go:1237`, `internal/onboarding/onboarding_test.go:290`,
  `internal/config/tokens_test.go:48`; copy that 5-line pattern into new packages,
  do not import across packages). Theme lookups read `XDG_STATE_HOME` — pin it the way
  `onboarding`'s `TestMain` does when a test is sensitive to the desktop palette.
- **No new dependencies.** Everything below needs only stdlib (`httptest`, `os.Pipe`)
  plus libraries already in go.mod (`slack-go` options, `charmbracelet/x/ansi`).
- **Reuse the existing idioms**: `newTest()` (app model on `source.NewMock()`),
  `WithSize`/`sized()`, `Key(m, "j")`, `Dump(m, w, h)`, `onboarding.Goto`,
  `ansi.Strip(m.View())` then `strings.Contains` on semantic content.
- Comments explain *why*; keep diffs tight; one purpose per test.
- Only one `TestMain` per package. Today only `internal/onboarding` has one — units
  adding tests there must NOT add another. Unit A2 (below) is the sole unit allowed to
  add a `TestMain` to `internal/app`.

## 3. Anti-patterns — tests that are forbidden

The user's requirement: tests **must be truthful and reliable**. A test that inflates
coverage but cannot fail is worse than no test. Concretely forbidden:

1. **No-panic tests** — calling a function and asserting nothing (or only `!= ""` /
   `!= nil`). If the only thing you can say about the output is that it exists, don't
   write the test.
2. **Assignment echoes** — asserting a struct field equals the value the test just
   assigned (e.g. `m.SetSize(10, 5); if m.width != 10`). `Tokens.Rotating()` is the
   boundary case: it is real boolean logic (`Refresh != "" || ExpiresAt > 0`), so its
   truth table is a legitimate test; `Prefs.Notifications()` likewise (`!= NotifyOff`
   means *unset defaults to on* — a documented migration decision).
3. **Implementation mirrors** — computing the expected value with the same arithmetic
   the code uses. Expected values must be derived independently: by hand
   (`blend("#000000","#ffffff",0.5)` → compute the composite yourself), from a fixture
   (`data.Mock()` message counts, counted by eye), or from a spec (RFC 7636 says the
   verifier is 43 chars of base64url).
4. **Golden blobs** — snapshotting a full rendered frame and comparing bytes. Any
   change "passes" by regeneration, ANSI noise makes diffs unreviewable. Instead assert
   *semantic facts* of a frame: this label present, that one absent, every line ≤ width,
   selected row carries the marker, background reasserted after each SGR reset.
5. **Over-mocking** — a stub so elaborate the test verifies the stub. The fake Slack
   HTTP server (unit S1) must return *fixed canned JSON* and assert *what the client
   sent*; it must not reimplement Slack semantics. When a test needs one failing
   method, embed the real `*source.Mock` in a tiny struct overriding that one method —
   never hand-roll a full 20-method `Source` recorder.
6. **Env-dependent tests** — anything whose result depends on what's installed
   (`notify-send`, an Omarchy theme, a busy port) must pin the environment (fake
   `lookPath`, fixture theme dir, pre-bind the port) so it fails only on a code
   regression, not on a machine difference.

Every work unit below names how its tests fail on a plausible regression. If an
implementer cannot preserve that property for a listed case, the case is dropped.

## 4. Production changes to make code testable

Minimal, behavior-preserving, and owned by the same unit that writes the tests against
them (so there is **no cross-unit sequencing**: each unit lands its seam and its tests
in one change). No other unit may rely on another unit's seam.

| # | File(s) | Change | Owner |
|---|---------|--------|-------|
| P1 | `internal/auth/oauth.go` | `authorizeURL`, `accessURL`: `const` → `var` (same values, unexported). Add `var browserOpen = openBrowser` and call `browserOpen(authURL)` in `Login`. | A-AUTH |
| P2 | `internal/notify/notify.go` | Add `var goos = runtime.GOOS` and `var lookPath = exec.LookPath`; `detect()` switches on `goos` and calls `lookPath`. | A-NOTIFY |
| P3 | `main.go`, `setup.go` | Extract the argv dispatch of `main()` into `func run(args []string) int` (printing as today; `main()` becomes `os.Exit`-thin — preserve the exact "no exit" vs `os.Exit(1)` behavior per branch). Change `promptClientID()` to `promptClientID(r io.Reader)`; `setup()` passes `os.Stdin`. | A-MAIN |

Nothing else. In particular: `internal/source` needs **no production change** —
`slack-go v0.25.0` has `slack.OptionAPIURL` and the tests live in package `source`, so
they can build `Slack` values with a client pointed at an `httptest.Server` directly.
`internal/doctor` needs none — `httpClient` is already a package `var` (its comment
says "a var so tests could stub it"); stub it with a `Client` whose `RoundTripper`
serves canned responses keyed on `req.URL.Path`, so the hardcoded `https://slack.com`
URLs never resolve.

## 5. Work units

Fourteen independent units. File ownership is exclusive: a unit only creates/edits the
files listed under **Owns**. Helpers defined in files a unit does not own (e.g.
`newTest`, `isolateConfigDir`, `withOmarchyTheme`, `Goto`) are used read-only.

---

### S1 — `internal/source`: the real Slack client over a fake Web API  *(largest single win: ~+280 stmts)*

**Owns:** new `internal/source/slack_web_test.go`, new `internal/source/socket_test.go`.
Does not touch `slack_test.go`, `mock.go`, or any non-test file.

**Technique:** one helper building the SUT hermetically:

```go
srv := httptest.NewServer(mux)            // mux: path → canned Slack JSON
s := NewSlack("xoxp-test")                // covers NewSlack
s.api = slack.New("xoxp-test", slack.OptionAPIURL(srv.URL+"/api/"))
```

Handlers `r.ParseForm()` and record what the client **sent**; assertions live on both
sides: request params sent, and the `data.*` values the method returned.

**Test (all in `slack_web_test.go`):**
- `Load`: canned `auth.test`, `users.list`, two-page `conversations.list` (first page
  returns a `next_cursor`). Assert: pagination actually followed (second-page channels
  present — fails if the cursor loop regresses); IM with `is_user_deleted` skipped;
  channel with `is_member:false` skipped; channels and DMs each sorted; DM named from
  users map, and a DM whose user is missing from `users.list` resolved via a canned
  `users.info` (`resolveDMNames`); with `SetGroupDMs(true)` the request's `types` form
  value includes `mpim` and the mpim row gets `mpimName`-cleaned name; without it, it
  doesn't. Handle index built (verify via `encodeMentions` on a loaded handle).
- `Unread`/`unreadFor`/`lastReadOf`/`setLastRead`: `conversations.info` returns
  `last_read=T`; history returns 5 messages — 2 older than T, 1 with a noise subtype
  (`channel_join`), 2 real and newer. Expected unread = 2, counted by hand from the
  fixture (fails if the subtype filter or the ts comparison regresses). Second call
  within TTL: assert `conversations.info` hit-count did **not** increase (cache), after
  a `MarkRead` the cached marker moves.
- `MarkRead` + `markErr`: assert `channel`/`ts` form values; canned `{"ok":false,
  "error":"missing_scope"}` → returned error mentions the scope remediation.
- `History`/`HistoryBefore`: `latest` param sent for Before; message order; `toMessage`
  derivation asserted on *derived* fields (Time `HH:MM` from a known unix ts, ReplyCount,
  `MentionsMe` true for `<@me>` in the canned text) — not an echo of the raw JSON.
- `Send`/`SendReply`: outgoing `@handle` encoded to `<@UID>` in the posted form text
  (independent expectation: the handle→ID pair the test seeded); `SendReply` carries
  `thread_ts`.
- `Replies`: multi-page thread (has_more/cursor) flattened.
- `React`: server answers `already_reacted` for add → client must issue
  `reactions.remove`, method returns `added=false`. Plain add returns `true`. Fails if
  the toggle fallback disappears.
- `Edit`, `Delete`, `Join`, `Joinable` (membership filter + pagination), `OpenDM`
  (`return_im` sent, conv mapped), `Leave`, `Snooze` (minutes param), `SetPresence`
  (all three statuses: `dnd` → `dnd.setSnooze`, `online` → `endSnooze`+`auto`, `away`),
  `SetStatusText`, `Presence` (one erroring user id → omitted, not failed),
  `Search` (hit mapping to `SearchHit`).
- `Upload`: `files.getUploadURLExternal` returns an upload URL **pointing back at the
  test server**; assert the file bytes arrive there and `completeUploadExternal` lists
  every file id plus the comment. Use `t.TempDir()` files.
- `Download` + `createUnique` + `sanitizeName`: `file.URL` points at the server; saved
  file content equals served bytes; a pre-existing name gets a uniquified path;
  `sanitizeName` pure table (path separators, empty → fallback).
- `IsRateLimited`: wrap a real `*slack.RateLimitedError` and a plain error. If any
  endpoint test needs a 429, send `Retry-After: 0` (the client has `OptionRetry(3)` and
  will sleep otherwise).
- Pure helpers not yet covered: `blocksText`, `richTextText`, `messageText`,
  `messageAuthor`, `toUser`, `tsParse` edge cases — table tests with hand-written
  expectations from the Slack block-kit JSON shapes.

**`socket_test.go`:** `socketAuthor` table (User > Username > BotID→"bot" > empty) and
`Events()` nil-before-start. Do **not** test `StartSocket`'s goroutines (§6).

**Worthless-test risk:** the fake server reimplementing Slack (anti-pattern 5) —
handlers stay dumb canned JSON; and asserting the response JSON round-trips (echo) —
always assert either a *derived* field or a *sent* parameter.

---

### S2 — `internal/source` mock + `internal/data`  *(~+95 stmts)*

**Owns:** new `internal/source/mock_test.go`, new `internal/data/data_test.go`.

The mock is not test scaffolding — it is the `go run .` dev backend and the substrate of
every app test; a silently broken mock weakens the entire suite. Test it as a backend:
`React` toggles (add → `true`, same again → `false`, and `History` shows the reaction
count move 0→1→0); `Edit`/`Delete` visible in subsequent `History`; `Join` returns the
conversation and removes it from `Joinable`; `Send`/`SendReply` appear in
`History`/`Replies` with the sent text; `UploadErr`/`DownloadErr` injection returns the
error *and* still records the call; `Snooze`/`Leave`/`OpenDM` record arguments.

`data_test.go`: referential integrity of the fixture — every `Message.UserID` across
`data.Mock().Messages` exists in `Users`; every message map key is a real conversation
ID; `Conversation(id)` found/not-found both branches; `Me()` returns the seeded
identity. These fail when someone edits the sample workspace and dangles a reference —
the exact bug class such a fixture invites.

**Worthless-test risk:** re-asserting fixture literals ("channel 1 is named general").
Only assert *relations* (integrity) and *behavior* (toggle, join-removes-from-joinable).

---

### A-AUTH — `internal/auth`: the OAuth round trip  *(~+50 stmts)*

**Owns:** `internal/auth/oauth.go` (seam P1 only), new `internal/auth/oauth_flow_test.go`.
`oauth_test.go` already covers the pure halves — leave it alone.

- `Exchange`: point `accessURL` at an `httptest.Server`; the handler asserts the form
  carries `client_id`, `code`, `redirect_uri`, `code_verifier` and **no
  `client_secret`** (that omission is a documented protocol requirement — this test
  fails if anyone "helpfully" adds the secret back). Canned success → tokens + team;
  `ExpiresIn` → `ExpiresAt` checked against the already-stubbable `now` var
  (fixed clock, independent arithmetic: `now + expires_in`).
- `Refresh`: same server; assert `grant_type=refresh_token`; nested and flat response
  shapes; `{"ok":false}` error propagation. (`parseRefresh` branch matrix is already
  covered — don't duplicate it; this is about the HTTP layer and form.)
- `Login` end-to-end on loopback: stub `browserOpen` to capture the URL; from the
  captured URL parse `state`, `code_challenge`, `redirect_uri`; then `http.Get` the
  redirect URI with `code=X&state=<captured>` — `Login` must return the tokens the fake
  `accessURL` server hands out, and the exchange request's `code_verifier` must hash
  (S256) to the captured `code_challenge` — a spec-derived assertion tying the two legs
  together. Error paths: `error=access_denied` param, wrong `state`, cancelled ctx.
  Loopback sockets only — hermetic. (If ports 9899–9903 are all busy the fallback error
  path of `listenLoopback` is what's exercised; also test that path deliberately by
  pre-binding `portFirst` and asserting the next port is chosen.)
- `page`: `httptest.NewRecorder`, assert content-type and the message text present.
- `Due`: boundary cases around `RefreshSkew` with a pinned `now` (due exactly at
  `expiresAt - skew`; non-rotating never due).

**Worthless-test risk:** asserting `AuthorizeURL` contains substrings the test itself
concatenated. It's already covered; don't add more of it. The Login test's value is the
*cross-leg consistency* (state echo, verifier↔challenge), which breaks on real bugs.

---

### A-DOCTOR — `internal/doctor`  *(~+89 stmts)*

**Owns:** `internal/doctor/doctor_test.go` (extend — no other unit touches it).

Swap `httpClient` for `&http.Client{Transport: rt}` where `rt` is a
`RoundTripper` func switching on `req.URL.Path` (`/api/auth.test`,
`/api/apps.connections.open`) — canned `*http.Response` from `httptest.NewRecorder`;
restore with `t.Cleanup`. Isolate config (`t.Setenv("XDG_CONFIG_HOME", t.TempDir())`)
and **clear** `SLACK_USER_TOKEN`/`SLACK_APP_TOKEN`/`SLACK_BOT_TOKEN` with `t.Setenv`.
Capture stdout via `os.Pipe` around `Run` (doctor prints with `fmt.Printf`).

Scenarios, asserting **exit code + specific lines** (the report is the product; its
lines are behavioral contract, but never snapshot the whole thing):
- no tokens anywhere → exit 1, line contains "no user token".
- tokens.json fixture + auth.test OK with `X-OAuth-Scopes` missing `reactions:write` →
  exit 0 and the line names both the scope and the feature ("a (react)") — fails if the
  `featureFor` wiring or header parsing (`splitScopes` comma format) regresses.
- env token shadowing a file token → the `!`-marked "env (overriding file!)" row.
- `reportRotation` states via fixtures: hand-pasted (no expiry), expiring-no-refresh
  (`✗ … run \`slack-tui login\``), expired, healthy (uses a future `ExpiresAt`).
- Socket Mode: unconfigured (`– not configured`), `connections.open` ok, and API error.
- auth.test HTTP failure → exit 1.
- Pure leftovers: `mask` (empty / short / no-dash / normal — expected strings written
  by hand), `fileExists` dir-vs-file, `dirHasFiles` (empty dir, dir with only subdirs,
  dir with a file), `envTokens` via `t.Setenv`.

**Worthless-test risk:** golden-diffing the whole report (anti-pattern 4), and
asserting `String()` returns its receiver. Assert one semantic line per scenario.

---

### A-CONFIG — `internal/config` + `internal/root`  *(~+84 stmts)*

**Owns:** new `internal/config/config_test.go`, new `internal/config/state_test.go`,
new `internal/root/root_test.go`. (Reuses `isolateConfigDir` from `tokens_test.go` —
same package; do **not** redefine it.)

`config`:
- `Load` with no file → `Defaults()`, `ok=false`. `Save` → `Load` round-trip with
  non-default values, `ok=true`.
- **The merge contract** (this is where a real bug already almost happened — the field
  comment documents it): write a raw JSON prefs file *without* the `notify` key (write
  the JSON by hand, don't marshal a struct) → loaded prefs keep `NotifyMentions`.
  Write `"notify":"off"` → off. Write `"group_dms":true` → true. Fails if `merge`
  starts treating empty-string as a value or the field becomes a bool.
- Corrupt JSON → defaults, `ok=false`. `Path()` under isolated dir. `defaultTheme`:
  with an Omarchy fixture (copy the `withOmarchyTheme` pattern from
  `onboarding_test.go:31` — pin `XDG_STATE_HOME`) → `theme.OmarchyName`; with an empty
  state dir → `"charcoal"`.
- `readConfigFile` legacy fallback: on Linux `legacyDir()` == `Dir()` when
  `XDG_CONFIG_HOME` is set, so the *distinct-dir* behavior is macOS-only — cover the
  fallback branch (primary read fails, legacy read runs) but do not pretend to assert
  the macOS semantics. Say so in a comment.
- `state.go`: `SaveState`/`LoadState` round-trip (drafts, recent, hidden); missing file
  → zero value + error; saved file mode is `0600` (drafts are message text — the
  comment says why; `os.Stat` the file).
- `Tokens.Rotating()` truth table (refresh-only / expiry-only / neither / both).

`root` (`package root` internal test):
- `New()` with isolated empty config → onboarding mode; after `config.Save` of
  onboarded prefs → loading mode. Assert observable behavior, not the enum:
  `View()` shows "connecting to slack…" vs the onboarding boot frame.
- `Update` walk: `WindowSizeMsg` then `appReadyMsg{app: app.NewWith(source.NewMock(),
  config.Defaults())}` → `View()` now renders the app at the remembered size
  (check a full-width frame line — fails if the stored size isn't applied);
  `onboarding.FinishedMsg` → loading view + non-nil cmd; `app.ReloadMsg` after the app
  is live → back to loading + non-nil cmd (also proves zero-risk `Shutdown` on the
  swap); `q`/`ctrl+c` during loading returns `tea.Quit`.
- Do not execute `loadApp` itself if config isn't isolated; with `isolateConfigDir` it
  builds the mock-backed app and may be exercised once (hermetic).

---

### A-THEME — `internal/theme`  *(~+41 stmts)*

**Owns:** new `internal/theme/theme_test.go`, new `internal/theme/render_test.go`.
Leaves `omarchy_test.go` alone.

- `blend`: one composite computed by hand (e.g. base `#000000`, overlay `#ffffff`,
  α 0.5 → `#808080`; plus an asymmetric pair verified with a calculator) and the
  invalid-hex passthrough. Independent expected values — never call `blend` to produce
  them.
- `Resolve`: unknown theme falls back to charcoal (assert a known charcoal token hex
  from the README "Design Tokens" table, the independent source); accent override
  changes only `Accent`; `"omarchy"` with fixture (pin `XDG_STATE_HOME`; copy the
  fixture pattern) resolves the fixture colors, and without a fixture falls back to
  charcoal.
- `Palette.Token`: all seven keys against README hexes for one theme + unknown → `Fg`.
- `Density`: `ParseDensity`/`String` round-trip incl. unknown input → comfortable;
  `MsgGap` 1/0 (that value *is* the feature — density in a terminal).
- `CycleFor`: `false` → equals `Cycle`; `true` → head is `OmarchyName` **and the
  package-level `Cycle` slice is unchanged afterwards** — catches the classic
  `append(Cycle, …)` aliasing regression.
- `DisplayName` both branches.
- `FillBg` (`render_test.go`): feed a lipgloss-styled string containing embedded
  `\x1b[0m` resets; assert (a) printable width == requested width for short and
  overlong inputs, (b) after every `\x1b[0m` in the output the bg sequence
  `\x1b[48;2;…` immediately follows except the final terminator — that re-assertion is
  the documented purpose of the function and disappears on regression, (c) invalid
  color → plain padded text with no escapes.

---

### A-COMP — `internal/ui/components` + pane top-up  *(~+220 stmts)*

**Owns:** new `internal/ui/components/components_test.go` (one file is fine — the
package is small), plus may extend `internal/ui/pane/pane_test.go` for the last two
branches (no other unit touches pane).

All pure `theme.Palette + data → string` renderers. Idiom: render, `ansi.Strip`, assert
semantic facts and width invariants (`lipgloss.Width(line) <= width` for every line).

- `windowStart`: pure math, full table — index at top/middle/bottom, total < maxRows,
  window pinned at both ends. Expected indexes worked out by hand.
- `Palette`: with 10 items, maxRows 4, index 8 → the *selected label* is present and
  the first item's label absent (proves windowing + selection, fails if either breaks);
  empty items → "no matches"; long labels truncated to box width; box corner glyphs on
  first/last line.
- `paletteRow`: selected row uses a different background than unselected (compare raw
  ANSI of the two — inequality of styling, not a blob).
- `Composer`: insert vs normal hint text ("↵ send" vs "i to write"); multi-line input
  → one continuation row per line, aligned; narrow width drops the hint before
  truncating input (render at a width where they can't both fit — assert hint gone,
  input intact).
- `Sidebar`: `BuildSideItems` — hidden map removes conversations but never headers;
  meta application sets unread/mention on the right conv (build the expectation from
  the fixture by hand); `SelectableIndexes` skips exactly the two header indexes;
  `SidebarBody`/`sideRow`: unread badge shown for unread conv, mention marker distinct,
  active conv row styled differently from cursor row.
- `StatusBar`/`H`: insert-mode tag present in insert, hints shown/hidden per flag,
  width respected. `TitleBar`: workspace name + handle + status glyph; pane count
  changes the segment. `ThreadScroll`; `PresenceColor`/`PresenceDot` mapping table
  (online/away/dnd/unknown → distinct outputs; unknown falls back — table with all
  keys, fails if a status maps to the wrong dot).
- `replyWho`/`replyPreview`/`plural` in messages.go: table tests ("1 reply", "3
  replies", who-list elision) — hand-written expectations.

**Worthless-test risk:** frame snapshots (forbidden) and asserting a style "contains
an escape code" without tying it to which row. Always anchor an assertion to a
semantic difference (selected vs not, hidden vs shown, truncated vs not).

---

### A-OB-VIEW — `internal/onboarding` rendering  *(~+330 stmts: view.go, wizard_view.go, trainer_view.go, typewriter.go)*

**Owns:** new `internal/onboarding/view_render_test.go`, new
`internal/onboarding/trainer_render_test.go`. Read-only reuse of `sized()`, `Goto`,
`Dump`, `Key`, `withOmarchyTheme`, existing `TestMain` (add none).

Drive with `Goto(New(), target)` + `Dump(m, 100, 30)` (and one narrow size, e.g.
72×20, per screen to hit the clamp/`contentW`/`panelDims` branches). Assert on
`ansi.Strip`ped output:

- `viewAuth`/`authRow`/`authColor`: the three auth options are listed; moving selection
  (`Key(m,"j")`) moves the marker between rows (assert marker on option 2's row and
  not on 1's — fails if selection rendering desyncs).
- `viewTokenForm`: three fields; focused field visibly distinct; entered token text
  appears.
- `viewLogin`, `viewLaunch`, `selRow`, `statusLabel`: workspace name shown after
  identity; status label maps per state.
- Wizard (`wizard_view.go`): `Goto(m, "wizard:theme")` → all five theme display names
  present + selected card marked; `wizard:accent` → accent names, `accentColor`
  mapping; `wizard:density` → both density cards with descriptions; status step rows;
  `stepRail` shows the current step highlighted and the done ones ticked; `viewFooter`
  key hints; `keycap`/`cardBox`/`swatchStrip` covered through these (don't unit-test
  glyph builders in isolation — assert their effect in the composed screen).
  `themeName`/`accentName` label tables.
  With `withOmarchyTheme` fixture: the theme step gains the "follow desktop" card.
- `distribute` and `wrapStyled` are logic, not layout: `distribute(total, n)` — widths
  sum exactly to total, max−min ≤ 1, hand-checked cases (10/3 → 4,3,3);
  `wrapStyled` — no output line wider than the limit (`lipgloss.Width`), words not
  split, styling survives across the wrap (raw string still contains the SGR intro).
- Trainer (`trainer_view.go` + `kc`/`drillInstruction`/`miniApp`/`miniMessage`/
  `miniPalette`/`miniText`/`stageColumn`/`railColumn`): `Goto(m, "wizard:keyboard:2")`
  → drills 0–1 shown done (tick), drill 2's instruction text shown, the mini-app pane
  for that drill rendered; completing a drill key via `Key` advances the shown drill —
  the instruction text changes to drill 3's (fails if trainer state → view wiring
  breaks).
- Typewriter (`typewriter.go`): construct via exported flow (`Goto(m, phaseBoot)` for
  a fast-forwarded view, and a fresh `New()` stepped with `Tick(m)` N times) —
  progressive reveal: after k ticks the k-th boot line is visible and k+2-th is not;
  `fastForward` shows the final line; `bootColor`/class styling: an "ok" line and a
  "dim" line render with different ANSI styling (compare raw); `oauthLines("slack")`
  contains the redirect URI (ties to `auth.RedirectURIs()[0]` — cross-checked
  independent value); `max0` table.

**Worthless-test risk:** full-frame goldens (forbidden) and progressive-reveal tests
that just assert "output grew" — always name the specific line that must (not) be
visible at tick k, with k chosen from the line list in the source fixture.

---

### A-OB-KEYS — `internal/onboarding` model, keys, dump  *(~+150 stmts: keys.go, onboarding.go, dump.go)*

**Owns:** new `internal/onboarding/keys_test.go`. Reuses `sized()`, `update`, `Key`,
`isolateConfigDir` (exists at `onboarding_test.go:290`), existing `TestMain`.

- `wizardKey` navigation: `j`/`k` clamp within `optionCount(step)`; `h`/`l` (and
  arrows) move steps via `next`/`back`, clamped at both rails; `applyOpt` on the theme
  step changes `m.themeName` and the *rendered* frame (assert the new theme's card
  marked — behavior, not field echo); `syncOpt` restores the selection when stepping
  back to a step with a prior choice.
- Finish: walk enter through the last step with `isolateConfigDir(t)`; read
  `prefs.json` back from the isolated dir and assert `onboarded:true` plus the exact
  theme/density chosen during the walk (independently known from the keys pressed);
  the returned cmd yields `FinishedMsg`. Fails if the wizard stops persisting a field —
  the prefs.json handoff is the module's whole contract.
- `tokenKey`: tab cycles the three fields (`focusToken`); enter with <4 chars stays;
  with a token advances and the token lands in the saved tokens file on finish
  (isolated).
- `identityKey`: empty handle blocked, non-empty advances (extends the existing test to
  the untested branches: esc, editing).
- `appSetupKey`: invalid client ID (an `A…` app id) rejected with the note set; valid
  ID saved (isolated oauth.json) and transitions toward OAuth — assert
  `m.phase == phaseOAuth` and `m.oauthRunning`, but **never execute the returned
  batch** (it contains `oauthCmd` → real network).
- `oauthDoneMsg` handling (synthesized, no network): error → `oauthErr` recorded and
  surfaced in the view; success with team "Acme Corp" → tokens.json written (isolated),
  workspace named per `workspaceName` — and `workspaceName` table-tested directly
  ("Acme Corp"→"acme-corp", empty→default; expectations hand-written from the
  lowercase/dash rule).
- `onboarding.go`: `speedMS` per phase; `onTick` advances the typewriter and emits the
  advance at completion (drive to `phaseAuth` exactly as `TestBootFastForwardThenAuth`
  does); `workspaceName`, `orDefault`, `clamp` tables; `Init` returns a tick cmd whose
  execution yields `tickMsg` (run the cmd once — deterministic).
- `dump.go`: `Goto` for every documented target string lands in the right phase
  (verify by a phase-distinctive string in `Dump`, e.g. the token form's field label —
  this guards the `--dump-ob` dev tool); `indexByte` table; `KeyC`/`Pump` covered by
  the flows above.

**Not tested here:** `startOAuth`'s `oauthCmd` body and `auth.Login` invocation (§6).

---

### A-APP-1 — `internal/app` overlays + view  *(~+180 stmts: help.go, statustext.go, confirm.go, view.go, dump.go, palette.go remainder)*

**Owns:** new `internal/app/overlays_test.go`, new `internal/app/view_more_test.go`.
Read-only reuse of `newTest`, `newSized`, `Key`, `WithSize`, `isolateConfigDir`.

- Help: `Key(m, "?")` opens; frame contains three spot-checked keymap rows (e.g.
  "dd", "ctrl+k", "quit"); `esc`/`q`/`enter` close; `ctrl+c` quits (cmd non-nil).
- `parseStatus` table: `":coffee: brb"` → (`:coffee:`, "brb"); `"no emoji"` → ("",
  text); `":unclosed rest"` → ("", whole string); whitespace trimming. Hand-written
  expectations from the *documented* split rule.
- Status-text flow: open via `openStatusText` (same package), type, enter → returned
  cmd executed against the mock yields `presenceMsg{nil}`; wrap the mock with a
  one-method override erroring `SetStatusText` → the error surfaces in `presenceMsg`
  and (after Update) in the frame. esc closes without a cmd.
- Confirm: `openConfirm` with an action that increments a counter — `y` runs it
  exactly once and closes; `n`/`esc` close with counter still 0 (fails if the guard on
  destructive actions regresses); overlay shows the confirm text; the `dd` delete flow
  reaches it end-to-end on the mock (message actually gone from `curMsgs` after `y`).
- `view.go`: width < 50 → the "needs a larger terminal" message and nothing else;
  `loadErr` set → banner with the error, and an over-width error text ends with "…";
  `copyToast` overlay clamped inside the frame at extreme `copyToastX`; thread open
  (`Key "enter"` on a threaded fixture message, as `thread_test.go` does) →
  `renderThread` output contains a reply's text and `dividerCol` renders between panes
  (a frame column of `│` at the divider x); every overlay flag renders through `View`
  (palette open + settings open + picker open reach their `overlay*` funcs).
- `overlayPalette`/`palette.go` remainder: open with `ctrl+k`, type a query that
  matches one command — only it is listed; `paletteMaxRows` windowing with many items.
- `Dump`: `lipgloss.Width` of the first rendered line == requested width for two sizes
  (guards the `--dump` dev flag without golden frames).

---

### A-APP-2 — `internal/app` live commands  *(~+120 stmts: live.go, app.go poll/notify paths)*

**Owns:** new `internal/app/live_more_test.go`. Does not touch `markread_test.go` or
`app_test.go`. **This unit alone may add `TestMain` to package app** if it needs to pin
`PATH` to an empty dir so `notify.detect` finds no notifier (keeps `applyEvent` tests
from spawning a real `notify-send` on desktop machines — hermeticity requirement).

- `unreadCmd`: run the returned cmd against the mock; expected counts derived by hand
  from the `data.Mock()` fixture (count the unreads in the fixture yourself). With a
  one-method override source returning `slack.RateLimitedError` for a chosen id: the
  result map omits it, includes the others fetched before the abort flag, and the
  overall cmd still returns (bounded, no hang — run under `t.Deadline` awareness).
- `unreadMsg` staleness: apply a `unreadMsg` whose `seq` predates a read (`m.readSeq`
  bumped) → the just-read conversation's badge must NOT resurrect (extends the
  markread_test.go story to the poll path; fails on regression of the seq guard).
- `markAllReadCmd`: ids with cached history use the cached ts; an id without cache
  triggers `History` then `MarkRead` (observe via recording override); first error is
  reported, later ones dropped.
- `lastRealTS`: table — trailing pending IDs skipped, all-pending → "" (the documented
  optimistic-send race; hand-built fixtures).
- `listenEvents`: fake `streamer` around a buffered channel → cmd returns
  `eventMsg{ev}`; closed channel → nil (the shutdown path).
- `applyEvent` via `Update(eventMsg{…})`: event for a non-active conv → unread badge +
  mention flag when the text mentions me; for the active conv → message appended to
  the visible pane (assert in `View`), no unread bump; thread reply event routed to
  the open thread.
- `dmIDs` head+tail windowing: build a model with `dmPollHead+dmPollTail+5` DMs;
  successive calls rotate the tail window (the bounded-poll behavior from v0.5.2 —
  derive the expected id sets by hand); `chanIDs` excludes the active conv.
- `presenceCmd` against the mock → `presenceUpdateMsg` statuses applied to sidebar
  dots; empty DM list → nil cmd.
- `historyCmd`/`refresh` against the mock → messages land in the model.

**Not tested:** `refreshTokenCmd` / `refreshIfDue` (needs auth's network seam across
packages), the `tea.Tick` wrapper trio, `bellCmd`, `listenEvents` re-arm timing (§6).

---

### A-APP-3 — `internal/app` interactions remainder  *(~+220 stmts: msgactions.go, picker.go, suggest.go, settings.go, slash.go, find.go, select.go, attach.go)*

**Owns:** new `internal/app/interactions_test.go`.

- `reactCmd` end-to-end on the mock: `a` opens the reaction picker
  (`openReactionPicker` — items populated from the emoji set), choosing one toggles on
  the mock and the message row shows the reaction; reacting again removes it.
- `openMsgLinks`/`openLinkPicker`: a message with two URLs → picker lists both;
  zero links → no picker. Do not execute `openURLCmd` (spawns a browser — §6); assert
  up to the cmd boundary.
- `slashSearch` + `slash.go` remainder: `/dm`, `/away`-style commands not yet covered
  by `slash_test.go` — each asserts its model effect on the mock (e.g. `/dm ada`
  activates ada's DM); search flow: `s`, query, enter → results from `Mock.Search`
  listed; selecting a hit jumps to the conversation.
- `settings.go`: `setStatus` cycles push a presence cmd (already partly covered) —
  add the GroupDMs toggle (flips pref, triggers reload cmd) and theme cycle
  persistence (`isolateConfigDir`; prefs.json content asserted).
- `find.go`: `/` in-channel find with a fixture needle → `n`/`N` wrap around
  (selection index sequence hand-computed); `overlayFind` shows query + match count.
- `select.go` `highlightVisible`: with a text selection active, the frame highlights
  exactly the selected span (assert styled region differs from the rest).
- `attach.go` remainder: `openAttach` browsing a `t.TempDir()` with known files —
  listing, `attachKey` navigation into a subdir, esc; `overlayAttach` shows entries
  (extends `attach_test.go` idiom; do not edit that file).
- `msgactions.go`: `reloadCmd`/`applyWorkspace` with `isolateConfigDir`: switching
  workspace writes the active name and emits `ReloadMsg`.

**Worthless-test risk (applies to all three app units):** driving `Update` and
asserting only "no error/cmd returned". Every case must pin an observable state change
(frame content, mock recording, or file content) that a plausible regression flips.

---

### A-NOTIFY — `internal/notify`  *(~+15 stmts)*

**Owns:** `internal/notify/notify.go` (seam P2 only), extend
`internal/notify/notify_test.go`.

Tests reset `once = sync.Once{}` (same package) around each detection case and restore
`goos`/`lookPath` with `t.Cleanup`:
- linux (`goos="linux"`): fake `lookPath` resolving `notify-send` to an executable
  recorder script written into `t.TempDir()` (`#!/bin/sh` appending `"$@"` to a file);
  `Send("#general", strings.Repeat("x", 200))` → recorded argv has
  `--app-name=slack-tui`, the title, and a body ≤ 140 runes ending in `…` (poll the
  file briefly — `run` is fire-and-forget). Fails if the app-name flag, argument
  order, or truncate-before-send regresses.
- darwin: `goos="darwin"` with `lookPath` faking `terminal-notifier` present →
  recorded argv uses `-title`/`-subtitle`/`-message`; only `osascript` present → the
  `-e` script contains the quoted body (ties `quote` to its call site).
- nothing found / `goos="windows"` → `Available()` false and `Send` a no-op (extends
  the existing safety test to real detection).

**Worthless-test risk:** asserting `detect` "set sendFn non-nil". The recorder-script
argv assertions are the truthful form; keep them.

---

### A-MAIN — repo-root `main` package  *(~+66 stmts)*

**Owns:** `main.go` + `setup.go` (refactor P3 only), extend `setup_test.go` or add
`main_test.go` (same package — this unit owns both test files).

All tests: `isolateConfigDir`-style `t.Setenv("XDG_CONFIG_HOME", t.TempDir())` **and**
`XDG_STATE_HOME` pinned — `--dump` builds `app.New()`, which must find no real tokens
(mock backend) and no desktop theme. Capture stdout with `os.Pipe` where output is the
contract.

- `versionString`: stamped (`version = "vX"` swapped and restored) vs dev fallback.
- `run(["slack-tui","--version"])` → prints "slack-tui " + version, exit 0.
- `run(["slack-tui","--dump","100x30","j,k"])` → frame on stdout with first-line
  printable width 100 (mock workspace content present, e.g. a fixture channel name) —
  guards the headless dev tool CONTRIBUTING relies on. Malformed size (`--dump bogus`)
  falls through without panic to… (mirror current behavior: it would start the TUI —
  so structure `run` to return before `tea.NewProgram` in test-reachable branches
  only; do NOT execute the interactive branch in tests).
- `run(["slack-tui","--dump-ob","90x28","wizard:theme"])` → theme cards present.
- `--workspace acme` sets `config.ActiveOverride` and strips the args (restore the
  global with `t.Cleanup`).
- `promptClientID(strings.NewReader(...))`: bad app-ID line then a valid ID → returns
  the valid one and printed the "looks like the App ID" hint; five garbage lines →
  error; EOF → error.
- `writeManifestFallback`: file lands in the isolated config dir, content byte-equal
  to the embedded manifest (which `setup_test.go` already validates against
  `auth.RedirectURIs`).

**Not tested:** `main()` itself, `login()` (real OAuth), `setup()`'s interactive
composition (clipboard + browser + login), `openBrowser`. §6.

---

## 6. What NOT to test (a test here would be theater)

- **`main()`, `login()`, `setup()` end-to-end** — thin compositions over the network
  flow and a real terminal program; their pieces are tested, the glue is unexercisable
  without faking so much that the test asserts the fakes.
- **Browser launchers**: `auth.openBrowser`/`OpenBrowser`, root `openBrowser` in
  setup.go, `app.openURLCmd` — per-OS `exec.Command` tables; a test would either spawn
  real processes or assert a mocked command string against itself.
- **`source.StartSocket`** — the socketmode goroutine loop needs a fake WebSocket
  server speaking Slack's socket protocol; the fake would dwarf the code and verify
  itself. Its pure edge (`socketAuthor`, event filtering constants) is covered in S1;
  the rest is exercised by `slack-tui doctor` guidance in the field.
- **`tea.Tick` wrappers** (`dmPollTick`, `chanPollTick`, `presencePollTick`,
  `pollTick`, `themeWatchTick`, onboarding `tick`) — asserting "returns a non-nil cmd"
  cannot fail meaningfully; the messages they deliver ARE tested by injecting them.
- **`app.refreshIfDue` / `refreshTokenCmd`** — would need auth's unexported endpoint
  var from another package; the parse/save halves are covered in auth and config.
- **`app.New` / `app.Init` / `app.Shutdown` / `bellCmd`** — environment-wiring
  (real config resolution, terminal bell, socket teardown). `NewWith` — the seam they
  wrap — is the tested constructor.
- **`onboarding.oauthCmd` / `startOAuth`'s cmd body** — wraps `auth.Login`; tested at
  the auth layer. The model-side transitions and `oauthDoneMsg` handling ARE tested.
- **`randHex`/`verifier` error branches** — `crypto/rand` failure is not fakeable
  without an abstraction that exists only for the test.
- **Fixture literals** (`data.Mock` field values), lipgloss/library behavior, and any
  full-frame golden snapshot, anywhere.

## 7. Sequencing and integration

- All fourteen units are **mutually independent**: disjoint file ownership (each
  production seam is owned by the unit testing it), so they can run fully in parallel
  and merge in any order. The only shared-package coordination rules: (1) only
  A-APP-2 may add a `TestMain` to package app; (2) nobody adds a second `TestMain` to
  onboarding; (3) helper functions are reused from the files that define them, never
  redefined or moved.
- Each unit's definition of done: `go vet ./... && go test ./...` clean from the repo
  root, its package's `go test -cover` at or above its target, zero tests that violate
  §3, and no test touching the network or real `$HOME`/config (spot-check: run the
  package's tests with `HOME=/nonexistent XDG_CONFIG_HOME= unset` sensitivity in mind —
  they must still pass under `env -i PATH=$PATH go test ./internal/<pkg>`-style
  isolation if challenged).
- Final integration step (any single agent, after merges): run the §0 measurement,
  paste the per-package `go tool cover -func` tail into the PR, and confirm
  `total ≥ 80.0%`. If short, the gap will be in app/onboarding/source — top up from
  the §1 uncovered map before touching anything else.

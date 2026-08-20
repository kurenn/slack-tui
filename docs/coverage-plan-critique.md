# Adversarial critique: docs/coverage-plan.md

Every claim below was verified against the working tree (main, v0.6.0) with the
plan's own measurement procedure. Re-measured baseline: **45.9%** total,
**5278** statements (`go test ./... -coverprofile -covermode=count`, own-package
tests). `go vet ./...` is clean today.

---

## BLOCKERS

### B1. Raw-ANSI styling assertions are unwritable as specified — no color profile is pinned, and the TestMain ownership rules forbid the fix

Under `go test`, stdout is a pipe, not a TTY, so termenv detects the Ascii
profile and **lipgloss emits zero escape sequences**. (The repo knows this:
`main.go:74` needs `FORCE_COLOR` + `lipgloss.SetColorProfile(termenv.TrueColor)`
just to make `--dump` emit color. No test file in the repo sets a profile —
verified by grep.)

Every "compare raw ANSI" assertion in the plan therefore compares two identical
unstyled strings and **passes vacuously** — the exact test theater §3 forbids:

- A-COMP: "selected row uses a different background than unselected (compare raw ANSI)" — `paletteRow`
- A-COMP: "active conv row styled differently from cursor row"
- A-OB-VIEW: "an 'ok' line and a 'dim' line render with different ANSI styling (compare raw)" — `bootColor`
- A-OB-VIEW: `wrapStyled` "styling survives across the wrap (raw string still contains the SGR intro)"
- A-APP-3: `highlightVisible` "styled region differs from the rest"

Carve-out: A-THEME's `FillBg` assertions are **fine** — `bgSeq`
(internal/theme/render.go:33) hand-builds `\x1b[48;2;…` with `fmt.Sprintf`,
independent of the lipgloss profile. But its test input ("a lipgloss-styled
string containing embedded `\x1b[0m` resets") must be built with literal escape
strings, not lipgloss, or it too will contain no resets.

**Fix (must land BEFORE the swarm — contradicts §7's "no sequencing"):** a
small foundation commit that adds `lipgloss.SetColorProfile(termenv.TrueColor)`
to a `TestMain` in each affected package (app, onboarding, components). This
cannot be done inside the current ownership rules: onboarding's `TestMain`
lives in `onboarding_test.go`, which **no unit owns**; app's `TestMain` is
reserved for A-APP-2, but A-APP-1 and A-APP-3 need the profile too. Do it once,
up front, in files the swarm then treats as read-only.

### B2. Hermeticity is still opt-in — the plan does not mandate an enforceable mechanism, which is how all three previous bugs happened

The plan says "every test that can touch config calls `isolateConfigDir(t)`".
That is per-test discipline, i.e. exactly what failed three times before. The
trap is structural: `newTest()` (app_test.go:18) → `config.Defaults()` →
`defaultTheme()` (config.go:181) → `theme.OmarchyAvailable()` **reads
`XDG_STATE_HOME` on every app-model construction**. Every new app, root, and
main test is desktop-theme-sensitive by default, whether or not it "touches
config".

The enforceable mechanism already exists in the repo:
`internal/onboarding/onboarding_test.go:24`'s `TestMain` pins
`XDG_STATE_HOME`, `XDG_CONFIG_HOME`, `HOME` to a temp dir and unsets
`SLACK_USER_TOKEN`/`SLACK_APP_TOKEN`/`SLACK_BOT_TOKEN`/`SLACK_CLIENT_ID`/
`SLACK_CLIENT_SECRET` for the whole package — its comment even narrates the
historical bugs.

**Fix:** the foundation commit (B1) replicates that TestMain in every package
the plan touches: app, config, root, main (repo root), theme, components,
doctor, source, auth, notify. Per-test `isolateConfigDir(t)` remains for tests
that need a *distinct* temp dir, but the package-level pin is the safety net
that makes a fourth incident impossible rather than merely discouraged.

### B3. A-MAIN will touch real tokens and the real network on the maintainer's machine

A-MAIN pins `XDG_CONFIG_HOME` and `XDG_STATE_HOME` but **never clears the
`SLACK_*` env vars**. `run(["--dump",…])` → `app.New()` (app.go:219) →
`config.LoadTokens()` (env overrides file) → `refreshIfDue` → if
`tok.User != ""`, a **real `source.NewSlack`** is built (so the frame shows an
empty workspace, not the mock — the "fixture channel name present" assertion
fails), and if App+Bot tokens are set, `sl.StartSocket(tok.App, tok.Bot)` opens
a **real Socket Mode connection from inside a test**. If only a rotating file
token exists, `refreshIfDue` can POST to the real `accessURL` and burn a
single-use refresh token. This is precisely the third historical bug ("real
tokens make onboarding skip a screen") transplanted into package main.

**Fix:** covered by B2's TestMain; additionally A-MAIN's spec text must say
"clear SLACK_* env" explicitly, as A-DOCTOR's already does.

### B4. The `--dump bogus` test case starts a real TUI

In today's `main()` (main.go:82-95), a malformed `--dump` size falls through
Sscanf and reaches `tea.NewProgram(root.New(), tea.WithAltScreen(), …).Run()`.
The plan simultaneously lists "Malformed size (`--dump bogus`)" as a test case
and hand-waves "structure `run` to return before `tea.NewProgram` in
test-reachable branches only" — but the bogus-size case IS the fall-through
branch; there is no way to test it and avoid the TUI under that structure. An
agent following the bullet literally hangs or machine-dependently fails the
suite.

**Fix:** specify P3 concretely: `run(args) (code int, launchTUI bool)` (or a
sentinel exit code) where `main()` alone constructs `tea.NewProgram` when
`launchTUI` is true. The bogus-size test then asserts `launchTUI == true` and
nothing else. Also see M3 (the `onboarding.AppManifest = manifest` wiring must
survive the extraction).

### B5. Same-package identifier collisions between parallel units

Three units write new test files in package `app`, two in `onboarding`, two in
`source`. The plan itself directs at least A-APP-1 (erroring `SetStatusText`),
A-APP-2 (rate-limited `Unread`, recording `History`), and A-APP-3 to each build
"a tiny struct overriding that one method" — plus each will want small local
helpers (`stripped`, `frame`, `newServer`…). Two agents independently choosing
`stubSource` or `errSource` produce a package that does not compile at merge.
File ownership is disjoint; **namespace ownership is not**.

**Fix:** mandate a per-unit prefix for every new top-level identifier in a
shared package (e.g. `ovl*` for A-APP-1, `live*` for A-APP-2, `ix*` for
A-APP-3, `web*` for S1, `mk*` for S2, `obv*`/`obk*` for the onboarding pair).

---

## CORRECTIONS

### C1. The numbers have drifted (arithmetic still survives — barely)

Measured now: total **45.9%** (plan: 45.6%); onboarding **1055 stmts @ 33.0%**
(plan: 1047 @ 31.1%); repo total **5278 stmts** (plan: 5270); 80% line = 4223
covered statements; `-coverpkg=./...` reads **51.2%** (plan: 50.9%). Redoing
the §1 table with current weights: targets sum to 4357 covered → **82.5%**,
buffer ≈ 134 statements. The conclusion holds; update the table so per-package
gates match reality. Note also that several targets are met *exactly* by the
units' claimed gains (auth 46+50=96, doctor 44+89=133, components 60+220=280 vs
278 needed) — zero per-package slack; every dropped "theater" case eats the
global buffer directly.

### C2. Stale line references

`isolateConfigDir` is at `onboarding_test.go:300`, not :290; `withOmarchyTheme`
at `onboarding_test.go:42`, not :31 (the package grew since the plan was
written). `app_test.go:1237` and `tokens_test.go:48` are correct.

### C3. The README "Design Tokens" table does not exist

A-THEME instructs (twice) to take expected charcoal hexes "from the README
'Design Tokens' table, the independent source". `grep -i 'design token'
README.md` → nothing; the only hex in README is a manifest
`background_color`. The actual sole source of the palette hexes is the
implementation itself (`internal/theme/theme.go:110`, `rawThemes["charcoal"]` —
e.g. fg `#c9d1d9`, accent `#56d4dd`). An agent sent to the README will stall or
quietly copy from the implementation while claiming independence.
**Fix:** either add the table to README in the foundation commit, or reword to
"pin literal hexes copied once into the test with a comment marking them a
frozen contract".

### C4. S1's "Time `HH:MM` from a known unix ts" assertion is timezone-dependent

`tsTime` (slack.go:989) formats `t.Hour()/t.Minute()` of a `time.Unix` value —
**local time**. The existing `TestTsTimeAndDay` (slack_test.go:45) deliberately
shape-matches `^\d{2}:\d{2}$` for exactly this reason. A literal expected
"14:53" passes in one timezone and fails in another — the env-dependent test
class §3.6 forbids, prescribed by the plan itself. **Fix:** shape-match like
the existing test, or pin `TZ` in source's (new, per B2) TestMain before any
`time` call.

### C5. The OptionRetry note contradicts S1's own helper

S1's snippet builds the test client as `slack.New("xoxp-test",
slack.OptionAPIURL(...))` — **without** `OptionRetry(3)`. With that client a
canned 429 returns `RateLimitedError` immediately; the warning "send
`Retry-After: 0` (the client has OptionRetry(3) and will sleep otherwise)"
applies only to a client built like production's `NewSlack` (slack.go:62). Say
which construction 429-related tests use; as written an agent will chase a
retry that never happens (or, worse, add the option and then be surprised by
sleeps).

### C6. Duplication the plan doesn't flag

`tsTime`/`tsDay`/`tsParse` are already covered (`TestTsTimeAndDay`); mock
`Presence` is already covered (`TestMockPresenceReturnsStatuses`,
`TestMockPresenceUnknownID` — note these live in `slack_test.go`, which S2 is
told not to touch, so S2 must not re-test them either); date separators are
covered in `components/messages_test.go`. Minor, but parallel agents duplicate
whatever the plan doesn't fence off.

### C7. Verified TRUE — for the record

- `slack.OptionAPIURL` exists (slack-go v0.25.0, slack.go:130), and **every**
  network call in internal/source goes through `s.api`: `MarkRead` →
  `api.MarkConversation` → `conversations.mark` (conversation.go:1032 in the
  module); `Download` → `api.GetFileContext` → `downloadFile` (misc.go:284, no
  URL restriction, honors the test-controlled `file.URL`); `Upload` →
  `GetUploadURLExternalContext`/`UploadToURL`/`CompleteUploadExternalContext`.
  No raw `http.Get`/`http.Post` in the package (the `net/http` import is for
  `DetectContentType`). The "zero production changes" claim is genuinely true.
- `doctor.httpClient` is a package var (doctor.go:40), `Run` returns an int and
  prints via fmt (stdout capture works), and the strings the unit asserts
  ("no user token", "env (overriding file!)", `featureFor`, `splitScopes`) all
  exist as described.
- auth: `now` is already a var (oauth.go:267); `authorizeURL`/`accessURL` are
  consts (oauth.go:51) — P1 is needed and sufficient; `exchangeForm` really
  omits `client_secret` (oauth.go:211); ports are 9899–9903 (portFirst 9899,
  portCount 5).
- P2 is needed as stated: `detect()` switches on `runtime.GOOS` and calls
  `exec.LookPath` directly (notify.go:46-70). notify truncation is 140 runes
  ending "…" with `--app-name=slack-tui` (notify.go:19,72,96).
- The §6 skip list is cheap, not load-bearing (measured): the excluded app
  functions total **88 uncovered stmts** (New 25, refreshIfDue 16,
  refreshTokenCmd 15, Init 10, openURLCmd 8, Shutdown/bellCmd/6 tick wrappers
  14), so app's coverage ceiling is 96.3% — the 85% target is arithmetically
  reachable. Onboarding's exclusions are 13 stmts; `StartSocket` is 31.
- Everything else spot-checked exists with the claimed semantics: `streamer` /
  `listenEvents` (live.go:287), `dmPollHead=10`/`dmPollTail=5`, `lastRealTS`,
  mock `UploadErr`/`DownloadErr` ("still records the call"), `merge`'s
  empty-string-means-unset Notify contract (config.go:167), state/tokens files
  0600, `parseStatus` table matches the code, help rows "dd"/"ctrl+k"/"quit",
  slash `dm`/`away`/`status`/`search`, settings GroupDMs toggle, root's
  loading-mode `q`/`ctrl+c` quit and "connecting to slack…" view, `Goto`
  targets incl. `wizard:…:N`, exported `Tick`/`Pump`, every S1 method name.

---

## RISKS

### R1. app is where the plan will actually miss

The three app units' claimed gains (+180/+120/+220 = +520) equal the needed
+518 almost exactly, and the residual uncovered mass sits in `Update` (59.9%)
and `normalKey` (59.0%) branch sprawl that no unit owns explicitly. Combined
with the exact-hit per-package targets (C1), app is the most likely source of a
final total in the 78–80% band. Mitigation: pre-agree that the §7 integration
agent tops up from `Update`/`normalKey` branches, and lower the acceptance
noise by restating app's gate as "≥ 84%" rather than pretending 85.0 is
guaranteed.

### R2. suggest.go: 43 uncovered statements counted, zero test cases written

A-APP-3's heading includes suggest.go in its +220, but no bullet mentions
`recomputeSuggest`, `mentionSuggestions`, `emojiSuggestions`, `acceptSuggest`,
`suggestKey`, or `overlaySuggest`. The agent will improvise or silently skip.
Add cases: `@` prefix filters to seeded handles, `:` prefix filters emoji
shortcodes, accept inserts the completion into the composer text (assert
composer content, not internal state).

### R3. Clipboard writes are the fourth hermeticity bug already in progress

Selection release calls `clipboard.WriteAll` (app.go:1226, error ignored) and
`y` yank calls it with error surfaced (app.go:1499-1508) — `atotto/clipboard`
execs xclip/wl-copy/pbcopy, so today's suite **already writes the developer's
real clipboard** (thread_test.go:126 even shrugs: "clipboard may fail in CI but
that's ok" — i.e. the test behaves differently per machine). The B2 TestMain's
PATH pin fixes this but *changes behavior*: the selection path still shows the
toast (error dropped), while `y` now takes the `flash("clipboard: …")` branch.
New copyToast/yank tests must be written against the pinned-PATH behavior, and
the existing suite must be verified green under the new TestMain **before** the
swarm branches off it.

### R4. A-APP-2's TestMain is conditional ("may add … if it needs to")

`notifyCmd` (live.go:651) is 70% covered today — existing tests already fire
`notify.Send`, which is fire-and-forget and pops a real `notify-send` on a
desktop Linux machine. A-APP-1 and A-APP-3 also traverse event/flash paths.
Make the TestMain unconditional and part of the foundation commit (B1/B2), not
a per-unit maybe.

### R5. Assorted flake vectors

- notify's `once = sync.Once{}` reset plus the fire-and-forget `run` goroutine
  makes the recorder-script assertions racy — poll with a deadline, restore
  `goos`/`lookPath` via `t.Cleanup` in LIFO order, and never `t.Parallel` in
  that package (no test in the repo uses `t.Parallel` today; keep it that way
  in packages with global seams).
- A-AUTH's port-exhaustion test must bind all five ports (9899–9903) in one
  test with cleanup, and will still flake if anything else on the machine holds
  one; consider asserting only the "next port chosen" case and taking the
  all-busy error branch as acceptable residual.
- Onboarding tick-count assertions ("k-th boot line visible after k ticks")
  depend on typewriter chars-per-tick; the plan is right to demand k be derived
  from the source line list — make agents read `typewriter.go` before choosing k.

### R6. §7's isolation "spot-check" is not a gate

"they must still pass under `env -i … if challenged`" — nobody challenges;
nothing runs it. With B2's TestMains this becomes redundant; without them it's
the same unenforced advice that already failed three times. If a belt-and-
suspenders check is wanted, add one CI step:
`env -u SLACK_USER_TOKEN -u SLACK_APP_TOKEN -u SLACK_BOT_TOKEN XDG_CONFIG_HOME=$(mktemp -d) XDG_STATE_HOME=$(mktemp -d) go test ./...`.

---

## MISSING

### M1. conversations.mark deserves end-to-end coverage matching its bug history

v0.5.3's production bug (commit 75dd588) was read state silently never reaching
Slack for DMs and private channels. S1 tests `MarkRead`'s form params and
A-APP-2 tests `markAllReadCmd`, but no listed case pins the app→source wiring
for the bug's actual shape: **reading a DM / private channel must invoke
`MarkRead` with that conversation's id**. Add one recording-source case per
conversation kind to A-APP-2 (extending the markread_test.go story), so the
regression that already shipped once cannot ship silently again.

### M2. The token-refresh path (31 stmts) is skipped on a false premise

§6 drops `refreshIfDue`/`refreshTokenCmd` because they "would need auth's
unexported endpoint var from another package". But the risky logic is not the
HTTP call (A-AUTH covers `Refresh` against httptest) — it's the app-side
decision and ordering, which `refreshIfDue`'s own comment calls out: the spent
refresh token is single-use, so **persist-before-return** is the contract; a
regression costs users a re-login. A one-line seam in app
(`var refreshToken = auth.Refresh`-style, same pattern as P1/P2, owned by
A-APP-2) makes testable: env-override skips refresh; not-due skips; due →
new tokens persisted to the isolated config **before** being returned; refresh
failure → old tokens returned and startup proceeds. This is load-bearing code
guarding real users' sessions; shipping the 80% milestone without it is the
plan optimizing the metric over the risk.

### M3. P3 must account for `onboarding.AppManifest = manifest` and the FORCE_COLOR global

`main()`'s first act wires the embedded manifest into onboarding
(main.go:44) and conditionally sets the global lipgloss profile (main.go:74).
The plan's run() extraction never says where these land. If the manifest wiring
doesn't run before `--dump-ob` app-setup rendering in tests, that screen
renders without a manifest — behavior change the tests would then enshrine.
Specify: manifest wiring happens inside `run` (or a shared init) so tests see
production behavior; FORCE_COLOR handling stays out of test-reachable paths or
is restored via `t.Cleanup`.

### M4. No merge protocol for the shared packages

Fourteen branches landing three-way into package app (and two-way into
onboarding/source) will conflict at minimum on nothing — *if* B5's naming rule
holds — but the plan should still say: merge order is arbitrary, each merge
re-runs `go vet ./... && go test ./...`, and the integrator (not the units)
resolves any collision by renaming per the unit prefix, never by deleting a
test.

### M5. TZ/locale pinning is absent everywhere

Beyond C4: `components/messages.go:21` and `tsDay` format dates; any new test
around date separators or day labels inherits machine TZ. One `TZ=UTC` line in
the foundation TestMains closes the whole class.

---

## WHAT'S GOOD (briefly)

- The central bet — OptionAPIURL makes internal/source testable with zero
  production changes — is verified true down to the module source, including
  uploads, downloads, and conversations.mark.
- §3's anti-pattern list is genuinely good and specific; §6's skips are cheap
  (88 excluded stmts in app) rather than excuse-making, and the per-file
  uncovered map matches the measured profile almost exactly.
- File-level ownership really is disjoint, and P1–P3 are correctly scoped,
  behavior-preserving seams owned by their testing units.

## Bottom line

The plan's facts are ~90% right and its arithmetic still clears 80% on paper.
But as written the swarm will (a) produce a family of vacuously-passing raw-ANSI
tests (B1), (b) reproduce the repo's signature hermeticity bug in package main
and possibly app (B2/B3), (c) hang or flake on the `--dump bogus` case (B4),
and (d) risk compile-breaking merges in app/onboarding/source (B5). All four
are fixed by ONE small foundation commit (TestMains: env pin + TZ + color
profile; P3 restructured around a launch sentinel; a naming rule added to §5)
that must land before any agent starts — the plan's "no sequencing needed"
claim is the single most dangerous sentence in it.

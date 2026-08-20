# Setting up slack-tui with an agent

Instructions for a coding agent (Claude Code, Cursor, …) asked to set up
slack-tui for its user. Follow them in order.

**Two steps need a human.** Slack's consent screens are the security boundary of
this whole flow — do not try to click through them, and do not ask the user for
their Slack password. Stop, say exactly what you need, and wait.

---

## 0. Check what's already there

```sh
slack-tui --version || echo "not installed"
slack-tui doctor 2>/dev/null | sed -n '/Tokens/,/^$/p'
```

If `doctor` reports a user token and `Auth` shows a workspace, setup is already
done — say so and stop. If the token is expired, slack-tui refreshes it on next
launch; that is not a reason to redo any of this.

## 1. Install

```sh
# macOS
brew install kurenn/tap/slack-tui

# macOS / Linux, no Go needed (verifies checksums)
curl -fsSL https://raw.githubusercontent.com/kurenn/slack-tui/main/install.sh | sh

# from source
go install github.com/kurenn/slack-tui@latest
```

The install script uses `/usr/local/bin`, falling back to `~/.local/bin` when
that isn't writable; `BIN_DIR=~/bin` overrides. Confirm the target is on `PATH`
before moving on — a successful install the user can't invoke is a failure.

## 2. Create the Slack app — **human required**

slack-tui isn't distributed through Slack, and an undistributed app installs
only in the workspace that owns it, so every user needs their own. There is no
way around this step: Slack's `?new_app=1&manifest_json=…` deep link no longer
prefills the create-app form, so the manifest must be pasted by hand.

Put the manifest where the user can paste it:

```sh
curl -fsSL https://raw.githubusercontent.com/kurenn/slack-tui/main/slack-app-manifest.json \
  | (wl-copy 2>/dev/null || pbcopy 2>/dev/null || xclip -selection clipboard 2>/dev/null) \
  && echo "manifest copied to clipboard"
```

Then tell the user, and wait:

> Go to https://api.slack.com/apps → **Create New App** → **From an app
> manifest** → pick your workspace → paste (it's on your clipboard) → **Next** →
> **Create**. Tell me when it's created.

## 3. Get the Client ID

From the app's **Basic Information → App Credentials**, you need the **Client
ID** only. It looks like `1234567890.9876543210` — two digit runs separated by a
dot.

- **Never ask for the Client Secret.** slack-tui doesn't use one and can't:
  sign-in is PKCE, and a PKCE client must not send a secret.
- The **App ID** (`A0B8KULBK8W`) sits directly above the Client ID and is the
  usual mis-paste. If you get something starting with `A`, ask again.

If you have authenticated browser control, you may read the Client ID yourself
from `https://api.slack.com/apps/<app-id>/general` — it is public information.
Do not read the secret field.

## 4. Sign in — **human required for the approval click**

```sh
SLACK_CLIENT_ID=<client-id> slack-tui login
```

This opens the browser and waits up to 3 minutes. Tell the user:

> Approve the permissions in the browser tab that just opened.

Expect `✓ Signed in to <workspace>`. To make it permanent so the env var isn't
needed again:

```sh
mkdir -p ~/.config/slack-tui
printf '{\n  "client_id": "%s"\n}\n' "<client-id>" > ~/.config/slack-tui/oauth.json
chmod 600 ~/.config/slack-tui/oauth.json
```

`slack-tui setup` does steps 2–4 interactively; prefer it when the user is at a
terminal, and use the manual sequence when driving non-interactively.

## 5. Verify

```sh
slack-tui doctor
```

Success looks like `✓ user`, `Auth ✓ <workspace> — <handle>`, and `✓ all
required scopes granted`. Then confirm it renders:

```sh
slack-tui --dump 100x30
```

## Optional: live unread (Socket Mode)

Only worth doing if the user asks for instant unread badges; without it they
refresh on a slow poll and everything else works.

OAuth cannot issue these two tokens — Slack refuses bot scopes on a loopback
redirect — so both are copied by hand from the app's admin page:

1. **OAuth & Permissions → Install to Workspace**, then copy the **Bot User
   OAuth Token** (`xoxb-…`).
2. **Basic Information → App-Level Tokens → Generate Token and Scopes**, scope
   `connections:write`, copy the `xapp-…` token.
3. Set both as `SLACK_BOT_TOKEN` / `SLACK_APP_TOKEN`, or paste them in
   onboarding's token form.
4. In Slack, `/invite @slack-tui` in each channel to stream.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `no Slack app configured` | No client ID. Steps 2–3. |
| `redirect_uri did not match` | The app's Redirect URLs are missing a port. All five of `http://localhost:9899…9903/callback` must be registered — re-paste the manifest. |
| `Must use PKCE to redirect to a non-web URI` | An old slack-tui. Upgrade; current versions always send PKCE. |
| `Bot scopes are not allowed…` | Same — upgrade. Bot tokens are never obtainable this way. |
| `no free loopback port in 9899–9903` | Something holds all five; usually a previous `slack-tui login` still running. |
| Sign-in denied | The workspace requires admin approval for third-party apps. The user must ask an admin; you cannot work around this. |
| Token expired | Expected — it rotates every ~12h and slack-tui refreshes it. Only re-run `login` if `doctor` reports no refresh token. |

## Rules

- Never ask for, log, or echo a Slack token or client secret. `doctor` masks
  them; keep it that way.
- `~/.config/slack-tui/tokens.json` is `0600`. Don't copy it elsewhere, don't
  print it.
- Don't click through Slack consent screens on the user's behalf, even with
  browser control.
- If a workspace blocks third-party apps, say so plainly. There is no
  workaround, and suggesting one would be wrong.

# matrix-absence-bot

A tiny Go program that logs into **your own** Matrix account and sends an
automatic "I'm away" reply to direct messages while it's running - at most
once per sender per day.

There is no schedule, presence check, or on/off toggle: **you are "absent"
for exactly as long as this process is running.** Start it before you step
away, stop it (Ctrl-C) when you're back. That's the whole model.

- Only replies in direct message (1:1) rooms, never group rooms.
- Never replies to your own messages (including its own prior replies).
- Sends at most one reply per sender per calendar day.
- Supports end-to-end encrypted DMs (which is most of them, by default).
- Also sets your Matrix presence (default: "unavailable"/"Away") for as long
  as it runs, so you don't show as available just because the bot is
  connected and polling `/sync`.

## 1. Create a dedicated login session for the bot

**Do not** grab the access token from a session you already use in Element
(or any other Matrix client). This bot manages its own end-to-end encryption
identity for its device, and it needs to generate that identity's keys
itself, on a device ID that has never uploaded keys before. If you reuse a
device ID that Element already initialized crypto for, the bot's crypto
startup will fail with a key-mismatch error - and worse, could confuse your
existing session.

Instead, create a **new** login session via a raw API call (this does not
touch or open any client, so no crypto identity is created except by the
bot itself on first run):

```bash
curl -s -XPOST "https://YOUR-HOMESERVER/_matrix/client/v3/login" \
  -H "Content-Type: application/json" -d '{
  "type": "m.login.password",
  "identifier": { "type": "m.id.user", "user": "YOUR-USERNAME" },
  "password": "YOUR-PASSWORD",
  "initial_device_display_name": "absence-bot"
}'
```

The response contains `access_token`, `device_id`, and `user_id` - copy
those into `config.yaml` (see below). Never log into this specific device ID
from Element or any other client afterwards.

If your homeserver requires SSO/other login flows instead of a password,
use the `/login` flow types it supports, or generate a token in your admin
panel if available - the important part is just that the resulting device
has never uploaded encryption keys before the bot's first run.

## 2. Generate a pickle key

This encrypts the bot's local crypto database at rest. Keep it secret, and
keep it stable across restarts (changing it makes the existing `crypto.db`
unreadable):

```bash
openssl rand -base64 32
```

## 3. (Recommended) Get your account's recovery key

Without this, your other devices may never trust the bot's device and may
never share the keys needed to decrypt messages in encrypted rooms - the
bot would silently fail to read (and reply to) any encrypted DM.

In Element: **Settings → Security & Privacy → Secure Backup**. If backup is
already set up, the recovery key was only shown once at creation time and
can't be re-displayed - you'd need to reset it (which invalidates the old
key on every device) to get a fresh one. If it's not set up yet, set it up
now and save the recovery key it shows you.

## 4. Configure

```bash
cp config.example.yaml config.yaml
```

Fill in `homeserver`, `user_id`, `access_token`, `device_id`, `pickle_key`,
and (recommended) `recovery_key`. Adjust `reply_message` to whatever you
want people to see, and `presence` if you don't want the default "Away"
status (see below). `config.yaml` is gitignored - it holds secrets.

## 5. Build & run

Building requires the `goolm` tag, which selects mautrix-go's pure-Go
olm/megolm implementation (no system libolm dependency). A `Makefile` bakes
this in for you:

```bash
make run      # go run, for trying it out
make build    # produces ./matrix-absence-bot
```

On first run, the bot uploads its device's encryption keys and (if
`recovery_key` is set) cross-signs itself so your other devices trust it.
Only messages sent *after* that point in *newly-shared* megolm sessions will
be decryptable - it can't retroactively decrypt history from before it
existed.

Building the crypto store also requires cgo (for the bundled SQLite driver)
- this needs a C compiler available (already the case on a normal macOS/
Linux dev machine; `CGO_ENABLED=1` must not be disabled).

## Usage

```bash
./matrix-absence-bot                      # uses ./config.yaml
./matrix-absence-bot -config /path/to/config.yaml
```

Leave it running while you're away; Ctrl-C (or `SIGTERM`) it when you're
back. Local state lives in `crypto.db` (encryption keys/sessions) and
`state.json` (which senders got a reply today) - both gitignored, both
safe to delete if you want to reset the bot's memory (deleting `crypto.db`
means re-doing device verification, though).

## Docker

A multi-stage `Dockerfile` is included, built on Alpine (builds with cgo +
the `goolm` tag internally, so you don't need Go installed locally). The
resulting runtime image is ~29 MB. Everything the bot needs to persist -
`config.yaml`, `crypto.db`, `state.json` - lives under `/data` in the
container; mount a host directory there.

```bash
mkdir -p data
cp config.example.yaml data/config.yaml   # then edit data/config.yaml

docker build -t matrix-absence-bot .
docker run --rm -it \
  --user "$(id -u):$(id -g)" \
  -v "$(pwd)/data:/data" \
  matrix-absence-bot
```

`--user "$(id -u):$(id -g)"` makes the container write `crypto.db`/
`state.json` as your own host user, so the mounted `data/` directory stays
readable/writable outside the container too - otherwise they'd be owned by
the container's built-in `bot` user (uid 10001).

Or with Docker Compose (`docker-compose.yml` is included, same volume
convention):

```bash
UID=$(id -u) GID=$(id -g) docker compose up -d --build
```

Or `make docker` builds the image locally with the same tag Compose expects
(`matrix-absence-bot`).

### Published image (GHCR)

`.github/workflows/docker.yml` builds and pushes the image to GitHub
Container Registry on every push to `main` (tag `latest`) and on version
tags (`vX.Y.Z`, tagged `X.Y.Z`/`X.Y`/`latest`) - no setup needed beyond
pushing to a GitHub repo, it authenticates with the automatic
`GITHUB_TOKEN`. Once published:

```bash
docker pull ghcr.io/<owner>/<repo>:latest
```

By default GHCR packages are private; make the package public (or use
`docker login ghcr.io`) if you want to pull without authenticating.

## Personalizing the reply message

`reply_message` supports these placeholders, filled in per-sender:

| Placeholder      | Value                                                        |
|-------------------|---------------------------------------------------------------|
| `{firstName}`    | First word of the sender's display name in the room          |
| `{displayName}`  | The sender's full display name in the room                    |
| `{username}`     | The sender's Matrix ID localpart (e.g. `alice` for `@alice:example.org`) |

Example: `"Hello {firstName}, I'm currently away and will get back to you as soon as I can."`

Falls back to the username wherever a display name isn't known yet (e.g. the
bot hasn't seen their profile before their first message).

## Presence

Every `/sync` request a client makes tells the homeserver `set_presence`,
and the Matrix spec's default for that is `online` - so a bot that's just
connected and long-polling `/sync`, with no presence handling of its own,
makes you show up as available to everyone the whole time it runs, no
matter what the auto-reply says. This bot sets `presence` (default
`"unavailable"`, shown as "Away" in most clients) on every sync request
instead, so your status matches reality. Set it to `"offline"` to look
fully offline, or `"online"` if you'd rather leave your real presence
alone and rely on the auto-reply only.

## How "one reply per sender per day" works

`state.json` maps each sender's user ID to the last calendar day (local
time) they got a reply. It's checked and updated on every qualifying
message and persisted immediately, so the limit survives restarts within
the same day.

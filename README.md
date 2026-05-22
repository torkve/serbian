# serbian — мобилни тренер за српски (C1)

Single-user PWA for a Russian native speaker drilling Serbian (Cyrillic) toward C1. Sessions are ≤2 minutes of mixed grammar, translation, and speaking, scheduled by SM-2 spaced repetition.

## Stack

- **Backend:** Go (single self-contained binary; embeds web assets, migrations, prompts).
- **Frontend:** plain JS ES modules, no build step.
- **DB:** SQLite via `modernc.org/sqlite` (pure Go, no CGo).
- **Speech-to-text:** external `whisper-server` (whisper.cpp) sidecar.
- **Optional LLM:** Anthropic API for task pre-generation and translation grading.
- **Push:** Web Push (VAPID) for daily reminders.

## Quick start

```bash
make build          # builds bin/{serbian,pregen,vapid}
make run            # starts the PWA on :8089
```

On first start, the server prints a one-time setup link:

```
listening on :8089
setup link: http://localhost:8089/setup?token=…
```

Open that URL once from the phone or laptop you want to authorize — it sets a long-lived auth cookie. Without a valid cookie, every page is 401.

The auth token is stored in `data/config.json` (created on first run). To re-auth, copy that URL from the log or regenerate by deleting the `auth_token` field in `data/config.json`.

## Docker (app + whisper)

A `docker compose` stack runs the PWA and a whisper.cpp STT sidecar in two
containers. Host directories `./data/` (DB, config, audio, backups) and
`./models/` (whisper ggml model files) are bind-mounted, so the container
flow is interchangeable with the native `make run` flow.

```bash
make whisper-model       # one-time, ~1.5 GB into ./models/
make docker-build        # build both images (~5-10 min first time)
make docker-up           # start app on :8089 + whisper on the internal network
make docker-logs         # find the /setup?token=... link
make docker-down         # stop the stack
```

Switch model size:

```bash
make whisper-model WHISPER_MODEL=large-v3
# update data/config.json whisper_url is unchanged; whisper-server uses /models/ggml-<MODEL>.bin
```

GPU (requires NVIDIA Container Toolkit on the host):

```bash
make docker-build GPU=1
make docker-up    GPU=1
```

The `app` image is a multi-stage build using `golang:1.25-alpine` and
`distroless/static-debian12` (~30 MB final). The host's pinned `$(GO_BIN)`
is **only** used by the native `make build`/`make run` flow — Docker is
fully self-contained.

To override the published host port:

```bash
make docker-up SERBIAN_PORT=9000
```

## Deploying behind nginx with HTTPS on a subpath

The app is designed to run unmodified behind a TLS-terminating reverse proxy, mounted at any subpath you choose (e.g. `https://example.com/serbian/`). iOS Safari's `getUserMedia` requires a secure context, so HTTPS at the proxy is mandatory for the speaking task.

**What makes this work out of the box:**

- All client-side URLs (HTML asset refs, ES module imports, `fetch()` calls, service worker registration, manifest `start_url`/`scope`) are relative — they resolve against whatever subpath the document is served from.
- The Go server reads `X-Forwarded-Prefix` from the proxy to emit the right `Set-Cookie Path` and the right `Location` on the one auth redirect (`/setup → /serbian/`).
- The server reads `X-Forwarded-Proto` to set the `Secure` cookie flag when the browser is on HTTPS, even though the app itself runs plain HTTP behind the proxy.
- The PWA service worker registers with a relative URL, so its scope is automatically the subpath.

**nginx config:** see `docs/nginx.conf.example`. The essential pieces:

```nginx
location = /serbian {
    return 301 /serbian/;          # trailing-slash so relative URLs resolve
}
location /serbian/ {
    proxy_pass http://127.0.0.1:8089/;   # trailing / strips the prefix
    proxy_set_header Host               $host;
    proxy_set_header X-Forwarded-Proto  $scheme;
    proxy_set_header X-Forwarded-Prefix /serbian;   # critical
    proxy_read_timeout 120s;             # whisper STT can take a while
    client_max_body_size 32m;            # audio uploads
}
```

Set `public_url` in `data/config.json` so the startup-log setup link shows the right URL:

```json
{
  "public_url": "https://example.com/serbian"
}
```

**Local smoke test of the proxy plumbing:**

```bash
make nginx-cert                       # one-time: self-signed cert
make docker-build
make docker-up WITH_NGINX=1           # app + whisper + nginx in front
# Open: https://localhost:8443/serbian/  (accept self-signed warning)
make docker-down WITH_NGINX=1
```

The `WITH_NGINX=1` flag layers `docker-compose.nginx.yml`, which adds an nginx container fronting the app and stops publishing the app's port directly.

## Whisper sidecar (required for speaking tasks)

The Go server forwards recorded audio to `http://127.0.0.1:8080/inference` by default (configurable via `whisper_url` in `data/config.json` or `WHISPER_URL` env var). Run `whisper-server` from [whisper.cpp](https://github.com/ggerganov/whisper.cpp) with the medium model:

```bash
# In your whisper.cpp checkout, after `make server`:
./server -m models/ggml-medium.bin --language sr --port 8080 \
         --convert  # decode opus/webm/mp4 client uploads
```

Browsers send `audio/webm;opus` (Chrome) or `audio/mp4` (Safari). Ensure your whisper.cpp build has ffmpeg support (`--convert` flag or built-in).

A systemd user service unit (recommended for a persistent setup) is left as an exercise — drop `whisper-server` into your tooling of choice.

## Push notifications

Generate a VAPID keypair once and copy it into `data/config.json`:

```bash
make vapid
# Add the printed vapid_public / vapid_private fields to data/config.json
```

Restart the server. The frontend home view will show "Укључи подсетнике" — tap it to subscribe (you must have added the PWA to your home screen on iOS 16.4+).

Default reminder times are 09:00 and 20:00 in `Europe/Belgrade`. Edit `time_zone` and `reminder_times` in `data/config.json` to change.

## Pre-generated tasks (optional, requires Anthropic API key)

The app ships with ~40 hand-written seed tasks (5 per kind). For bulk content:

1. Get an API key at https://console.anthropic.com.
2. Add it to `data/config.json` as `"anthropic_api_key": "sk-ant-…"`, or set `ANTHROPIC_API_KEY=…` in the environment.
3. Generate batches:

```bash
make pregen ARGS="--kind cloze --topic cases.instrumental --difficulty 4 --count 30"
make pregen ARGS="--kind tr_ru_sr --difficulty 4 --count 30"
# ... etc
```

Use `--dry-run` to inspect before inserting:

```bash
make pregen ARGS="--kind aspect --count 5 --dry-run"
```

Review the generated content at `/admin/review` (authenticated). Use **Означи** to exclude bad items from sessions without deleting; **Обриши** to remove them entirely.

## Live translation grading (optional)

If an Anthropic API key is configured, translation answers that fuzzy-match in the "ambiguous" band (similarity 0.6–0.9) are upgraded by a live Claude call. Daily budget defaults to 100 calls (`daily_claude_budget` in config). When the budget is exhausted, falls back to fuzzy. Every call is logged to the `claude_calls` table.

## Multiple users

The same instance serves multiple users; each has their own SRS state, sessions, attempts, and push subscriptions. Task content (the question bank) is shared. There is no admin/regular role distinction — anyone with a valid token has full access including the admin review page.

### Native (running via `make run`)

```bash
./bin/serbian -add-user alice      # creates the user, prints a setup link
./bin/serbian -list-users          # tabular dump: id, name, created, last_seen, token
./bin/serbian -delete-user alice   # cascade-deletes the user + their per-user rows
```

### Docker (running via `make docker-up`)

User-management commands are one-shot invocations of the same binary against the bind-mounted DB. The running `app` container is left alone — these spin up a short-lived container that exits as soon as it prints the result.

```bash
docker compose run --rm --entrypoint /app/serbian app -add-user alice
docker compose run --rm --entrypoint /app/serbian app -list-users
docker compose run --rm --entrypoint /app/serbian app -delete-user alice
```

(`--entrypoint /app/serbian` is needed because the image's default ENTRYPOINT bakes in `-config /app/data/config.json -backup-dir /app/data/backups`; this override drops those flags and the binary falls back to the same paths via its built-in defaults relative to `WORKDIR /app`.)

If you've enabled the local nginx overlay, add `-f docker-compose.nginx.yml` so the volumes resolve identically — though for a one-shot CLI you can equally drop the overlay:

```bash
docker compose -f docker-compose.yml -f docker-compose.nginx.yml \
    run --rm --entrypoint /app/serbian app -list-users
```

### What the setup link looks like

```
created user #2 alice
setup link: https://example.com/serbian/setup?token=I3a5IJcu95gKv088mpQ6iBi-_P1UVZU9
```

The URL prefix comes from `public_url` in `data/config.json` (recommended for the prod setup behind nginx) or falls back to `http://localhost<port>`. Send the link to the new user once over a secure channel; they open it on their phone, the cookie gets set, and they're authenticated forever (or until you `-delete-user`).

### Migrating from a single-user install

The first time the binary starts on a pre-multi-user DB, the existing owner is created automatically as user #1 using the pre-existing `auth_token` from `data/config.json`. All prior `srs_state`/`sessions`/`attempts`/`push_subs` rows are attached to that user. The owner's existing cookie keeps working unchanged — no re-setup needed.

## Backups

```bash
make backup        # one-shot snapshot to data/backups/serbian-YYYY-MM-DD.db
```

The running server also takes an automatic daily snapshot ~04:30 local time and retains the most recent 7.

## Layout

```
cmd/serbian   # PWA server (embeds web/, migrations/, prompts/)
cmd/pregen    # Anthropic-API task generator
cmd/vapid     # one-shot VAPID keypair generator
internal/
  server/     # routes, middleware, auth, session+speak+push+admin APIs
  store/      # sqlite open, migrations, backup
  tasks/      # task types, SM-2, scheduler, grading
  stt/        # HTTP client for whisper-server
  llm/        # Anthropic SDK client, pregen prompts, translation grader
  push/       # webpush-go sender, in-process scheduler
  config/     # JSON + env config loader
web/          # static PWA assets, all in Cyrillic
migrations/   # 0001_init.sql, 0002_seed.sql
prompts/      # Claude prompt templates (one per kind)
data/         # runtime: config.json, serbian.db, backups/ (gitignored)
```

## API surface (all auth-required except /setup and /healthz)

| Method | Path                                    | Purpose                                |
| ------ | --------------------------------------- | -------------------------------------- |
| GET    | `/healthz`                              | Liveness probe                         |
| GET    | `/setup?token=…`                        | One-time auth cookie set               |
| POST   | `/api/session/start`                    | Compose a ≤2-min session               |
| POST   | `/api/session/{id}/attempt`             | Submit text answer                     |
| POST   | `/api/session/{id}/speak`               | Submit audio (multipart)               |
| POST   | `/api/session/{id}/end`                 | Mark session done; returns summary     |
| GET    | `/api/push/vapid`                       | VAPID public key                       |
| POST   | `/api/push/subscribe`                   | Save subscription                      |
| POST   | `/api/push/unsubscribe`                 | Drop subscription                      |
| POST   | `/api/push/test`                        | Fire a test push to all subscriptions  |
| GET    | `/admin/review`                         | Admin UI                               |
| GET    | `/api/admin/tasks?source=…`             | List tasks for review                  |
| POST   | `/api/admin/tasks/{id}/flag`            | Toggle flagged (hides from sessions)   |
| DELETE | `/api/admin/tasks/{id}`                 | Permanently delete                     |

## Config reference (`data/config.json`)

```json
{
  "addr": ":8089",
  "db_path": "data/serbian.db",
  "audio_dir": "data/audio",
  "auth_token": "auto-generated on first run",
  "anthropic_api_key": "sk-ant-…",
  "anthropic_model": "claude-opus-4-7",
  "whisper_url": "http://127.0.0.1:8080/inference",
  "whisper_language": "sr",
  "vapid_public": "…",
  "vapid_private": "…",
  "vapid_subject": "mailto:user@localhost",
  "time_zone": "Europe/Belgrade",
  "reminder_times": ["09:00", "20:00"],
  "daily_claude_budget": 100
}
```

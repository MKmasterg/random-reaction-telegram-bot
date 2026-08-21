# Random Reaction Telegram Bot

A small Go bot that occasionally replies to ordinary Telegram group messages with mixed Persian and English reactions. It runs as event-driven HTTP functions, a standalone webhook server, or a local polling process. Every mode uses Redis for shared per-group cooldowns.

## Behavior

- `/reaction` returns a random general reaction.
- `/personal` includes the sender's display name.
- `/start` and `/help` explain usage and Telegram privacy requirements.
- Ordinary group messages have a 2% reply chance and a five-minute cooldown by default.
- Commands bypass probability and cooldown.

The bot ignores channels, service events, bots, media, unsupported chats, and unknown commands. Replies are plain text without a parse mode.

Webhook processing is at least once: failed processing returns `503` so Telegram can retry. Automatic probability is stable for a given update, while update deduplication is process-local, so retries after timeouts or on another function instance can produce duplicate replies.

## Telegram setup

1. Open [@BotFather](https://t.me/BotFather) and run `/newbot`.
2. Store the token as `TELEGRAM_BOT_TOKEN`.
3. For automatic reactions, use `/setprivacy` and disable Group Privacy Mode, or make the bot a group administrator.
4. Remove and re-add the bot to existing groups after changing privacy mode.

Use a separate development bot token locally. Polling removes that token's existing webhook.

## Configuration

| Variable | Required | Default | Description |
|---|---:|---:|---|
| `TELEGRAM_BOT_TOKEN` | Yes | — | BotFather token. |
| `REDIS_URL` | Yes | — | `redis://` locally or `rediss://` for managed TLS Redis. May contain credentials; never log it. |
| `REDIS_KEY_PREFIX` | Yes | — | Shared by replicas of one environment and different across development, preview, and production. |
| `REPLY_PROBABILITY` | No | `0.02` | Number greater than `0` and at most `1`. |
| `GROUP_COOLDOWN` | No | `5m` | Go duration of at least `1ms`. |
| `BOT_TRANSPORT` | No | `polling` | Standalone mode: `polling` or `webhook`. |
| `PORT` | No | `8080` | Standalone webhook listener port. |
| `PUBLIC_BASE_URL` | Registration/standalone webhook | — | Public HTTPS origin without a path. |
| `WEBHOOK_SECRET` | Webhook | — | 1–256 letters, digits, `_`, or `-`. |

Redis keys use `<REDIS_KEY_PREFIX>:cooldown:<chat_id>`. Do not share a prefix between development and production bots.

## Local development

Requirements: Go 1.26, Docker, and a development bot token.

```sh
cp .env.example .env
docker compose up -d redis
set -a
source .env
set +a
make run
```

The default transport is polling. Redis is required for automatic reactions; explicit commands do not read Redis.

## Event-driven function deployment

The repository provides two HTTP function entry points:

- `api/webhook/index.go` handles Telegram updates at `/api/webhook`.
- `api/healthz/index.go` reports configuration and Redis readiness at `/api/healthz`.

Both export a standard `http.HandlerFunc`-compatible `Handler`. They delegate through the public `functions` bridge so generated runtime wrappers do not import Go `internal` packages directly. If a hosting platform uses a different function layout, keep the adapter thin and reuse the same bridge.

1. Provision a standard Redis service reachable from the functions. Prefer a TLS `rediss://` connection for remote Redis.
2. Configure `TELEGRAM_BOT_TOKEN`, `REDIS_URL`, `REDIS_KEY_PREFIX`, and `WEBHOOK_SECRET` in the deployment environment.
3. Deploy both function entry points.
4. Verify `GET https://your-project.example/api/healthz` returns `200`.
5. Register the stable public URL from a trusted machine:

```sh
set -a
source .env.production
set +a
BOT_TRANSPORT=webhook PUBLIC_BASE_URL=https://your-project.example make register-webhook
```

Telegram sends updates to `POST /api/webhook`. The endpoint must be public, use HTTPS, and allow Telegram's webhook requests through any access-control layer. Do not register temporary deployment URLs.

The registrar validates configuration, pings Redis, calls the deployed `/api/healthz`, and checks Telegram's `SetWebhook` result before reporting success. A failed or protected deployment does not replace the active webhook.

## Standalone webhook and Docker

The standalone server exposes `POST /api/webhook`, the compatibility alias `POST /telegram/webhook`, and `GET /healthz`. It registers `/api/webhook` at startup.

Run the full Compose stack with values from `.env`:

```sh
docker compose up --build bot
```

The image remains usable without Compose when `REDIS_URL` points to a reachable Redis service:

```sh
docker build -t random-reaction-telegram-bot .
docker run --rm --env-file .env -p 8080:8080 random-reaction-telegram-bot
```

Compose publishes Redis and the bot only on loopback. The bot port is useful for local webhook testing; polling mode does not need inbound traffic.

## Content and checks

Reaction text lives in [`internal/content/reactions.json`](internal/content/reactions.json) and is embedded into the binary. Changes require a rebuild.

```sh
make fmt-check
make vet
make test-race
make build
make redis-up
make test-integration
make docker-build
```

## Failure behavior

- Redis unavailable: automatic candidates fail closed; webhook requests return `503`, while explicit commands remain Redis-independent.
- Telegram send failure: the Redis reservation is released and webhook delivery is retried.
- Ambiguous timeout: a retry can duplicate a reply; exact-once delivery is out of scope.
- Process restart: Redis cooldowns survive, while recent webhook update IDs do not.
- Function runtime issue: deploy the same application through the standalone Docker webhook mode.

Logs include update IDs, chat IDs, decisions, and errors. They do not intentionally include tokens, Redis URLs, or message bodies.

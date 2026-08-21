# Random Reaction Telegram Bot

A small, stateless Go bot that occasionally replies to ordinary messages in Telegram groups. Reactions are selected from embedded Persian and English content, and explicit commands are always available.

The bot supports local long polling and HTTPS webhooks. Per-group cooldowns and recent webhook update IDs live in memory and reset whenever the process restarts.

## Behavior

- `/reaction` replies with a random general reaction.
- `/personal` replies with a reaction using the sender's display name.
- `/start` and `/help` explain the commands and Telegram privacy requirement.
- Private chats only return the explanation for these known commands.
- Ordinary group messages have a 2% reply chance by default, followed by a five-minute cooldown for that group.
- Commands bypass both the random chance and the cooldown.

The bot ignores channels, service events, messages from bots, commands it does not recognize, media, and non-text updates. It sends plain text and does not enable a Telegram parse mode.

## Create and configure a Telegram bot

1. Open [@BotFather](https://t.me/BotFather) and run `/newbot`.
2. Copy the token into `TELEGRAM_BOT_TOKEN`. Treat it like a password.
3. For automatic replies, use `/setprivacy` in BotFather and choose **Disable** for this bot. Alternatively, make the bot a group administrator.
4. Remove and re-add the bot to existing groups after changing privacy mode.

Telegram's Group Privacy Mode prevents a normal group bot from receiving most ordinary messages. Commands can still work while automatic reactions appear broken if privacy mode remains enabled. See [Telegram's bot privacy documentation](https://core.telegram.org/bots/features#privacy-mode).

Use a separate development bot token for local polling. Polling startup removes an existing webhook, so using a deployed bot's token locally will disconnect that deployment.

## Local setup

Requirements:

- Go 1.26
- A Telegram bot token

Copy the environment template and add the development token:

```sh
cp .env.example .env
```

Load it in the current shell and run the bot:

```sh
set -a
source .env
set +a
make run
```

Long polling is the default transport. Stop it with `Ctrl+C`; `SIGINT` and `SIGTERM` both trigger graceful shutdown.

## Configuration

| Variable | Required | Default | Description |
|---|---:|---:|---|
| `TELEGRAM_BOT_TOKEN` | Yes | — | Secret token issued by BotFather. |
| `BOT_TRANSPORT` | No | `polling` | `polling` or `webhook`. |
| `REPLY_PROBABILITY` | No | `0.02` | Number greater than `0` and at most `1`. |
| `GROUP_COOLDOWN` | No | `5m` | Positive [Go duration](https://pkg.go.dev/time#ParseDuration). |
| `PORT` | No | `8080` | HTTP listener port in webhook mode. |
| `PUBLIC_BASE_URL` | Webhook only | — | Public HTTPS origin, without the webhook path. |
| `WEBHOOK_SECRET` | Webhook only | — | Shared secret using 1–256 letters, digits, `_`, or `-`. |

For webhook mode, the service binds to `0.0.0.0:$PORT`, accepts Telegram updates at `POST /telegram/webhook`, and exposes `GET /healthz`. At startup it registers `${PUBLIC_BASE_URL}/telegram/webhook` and requests message updates only. TLS is expected to terminate at the public HTTPS endpoint.

Webhook processing uses at-least-once attempts. Successfully handled update IDs are retained in bounded memory; failed sends return HTTP `503` and release the ID so Telegram can retry it. A retry can produce a duplicate reply after an ambiguous timeout, and the retained IDs reset whenever the process restarts.

Example webhook settings:

```dotenv
BOT_TRANSPORT=webhook
PUBLIC_BASE_URL=https://bot.example.com
WEBHOOK_SECRET=replace-with-a-long-random-value
PORT=8080
```

Do not commit real values. `.env` and common secret file variants are ignored.

## Edit reactions

Reaction text lives in [`internal/content/reactions.json`](internal/content/reactions.json) and is embedded in the executable. Changes require a rebuild.

The schema is:

```json
{
  "general_reactions": ["..."],
  "personal_groups": [
    {
      "template": "reaction for {name} when {action}",
      "actions": ["..."]
    }
  ]
}
```

Both lists and every action must be nonempty. Each personal template must contain `{name}` and `{action}`. Invalid content stops the bot during startup and is covered by tests.

## Build and verify

```sh
make fmt-check
make vet
make test
make test-race
make build
```

`make check` runs formatting verification, vetting, and race-enabled tests. The compiled binary is written to `bin/random-reaction-telegram-bot`.

## Docker

Build the multi-stage, non-root image:

```sh
docker build -t random-reaction-telegram-bot .
```

Run it with long polling:

```sh
docker run --rm --env-file .env random-reaction-telegram-bot
```

For webhook mode, publish the configured port and supply the webhook environment values through the platform's secret manager:

```sh
docker run --rm --env-file .env -p 8080:8080 random-reaction-telegram-bot
```

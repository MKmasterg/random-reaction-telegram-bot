# Agent guide

## Project intent

This repository contains a Telegram group bot written in Go 1.26. It supports event-driven HTTP functions, standalone webhooks, and local polling. Every production mode uses the same Redis-backed group cooldown; do not add an in-memory production fallback.

Function adapters belong under `api`. Keep them thin, keep domain packages and Redis state portable, and do not add deployment-provider manifests, badges, or provider-specific instructions to tracked files.

## Package boundaries

- `api`: thin event-driven HTTP function adapters.
- `cmd/bot`: signals and standalone transport selection.
- `cmd/register-webhook`: readiness-gated Telegram webhook registration.
- `internal/application`: shared dependency wiring for every runtime.
- `internal/config`: environment parsing and validation.
- `internal/content`: embedded JSON and validation.
- `internal/domain`: Telegram-independent update types.
- `internal/handler`: commands, exclusions, probability, cooldown policy, and small interfaces.
- `internal/reaction`: content selection and display-name normalization.
- `internal/redisstore`: namespaced atomic cooldown reservations.
- `internal/telegram`: Telegram conversion and delivery.
- `internal/transport`: polling, standalone HTTP, and webhook validation.

Keep handler tests network-free. Test Redis scripts against a real isolated Redis service as well as unit fakes. Occasional duplicate webhook replies are accepted; durable update deduplication is out of scope.

## Content schema

`internal/content/reactions.json` is UTF-8 and embedded into the binary. It requires nonempty `general_reactions`, nonempty `personal_groups`, `{name}` and `{action}` in every template, and nonempty actions. Keep content playful without slurs, protected-class targeting, explicit sexual material, or hostile personal attacks.

## Behavior

- `/reaction`: general reaction.
- `/personal`: personalized reaction.
- `/start` and `/help`: usage and privacy explanation.
- Commands bypass Redis, probability, and cooldown.
- Automatic replies apply only to ordinary group and supergroup text messages.
- Use plain Telegram text without a parse mode.

## Required checks

```sh
make fmt-check
make vet
make test-race
make build
make redis-up
make test-integration
make docker-build
```

## Secrets

Never commit, print, or log Telegram tokens, webhook secrets, Redis URLs, `.env` contents, or real production URLs. `REDIS_KEY_PREFIX` is not secret but must differ between environments. Polling deletes the token's existing webhook, so use a separate development bot.

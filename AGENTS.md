# Agent guide

## Project intent

This repository contains a stateless Telegram group bot written in Go 1.26. It provides explicit reaction commands and low-probability automatic replies with an in-memory per-group cooldown. Keep the implementation small, provider-neutral, and free of databases, analytics, media handling, and per-group configuration unless the task explicitly changes scope.

Do not add deployment-provider names, badges, manifests, or provider-specific setup instructions to tracked files.

## Package boundaries

- `cmd/bot`: dependency wiring, signals, and transport selection only.
- `internal/config`: environment parsing and validation.
- `internal/content`: embedded JSON schema, parsing, and startup validation.
- `internal/domain`: Telegram-independent update types.
- `internal/reaction`: random selection and display-name normalization.
- `internal/handler`: commands, exclusions, probability, cooldown, and reply policy.
- `internal/telegram`: Telegram SDK conversion and message delivery.
- `internal/transport`: long-polling startup, webhook registration, HTTP validation, and graceful shutdown.

Keep domain tests network-free by depending on the existing small interfaces. Protect shared webhook state against concurrent requests.

## Content schema

`internal/content/reactions.json` is UTF-8 and embedded into the binary. It contains:

- nonempty `general_reactions` strings;
- nonempty `personal_groups`;
- a `template` in each group containing both `{name}` and `{action}`;
- nonempty `actions` strings in each group.

Content should stay playful and may mix Persian and English. Do not add slurs, protected-class targeting, explicit sexual material, or genuinely hostile personal attacks.

## Commands and behavior

- `/reaction`: general reaction.
- `/personal`: personalized reaction.
- `/start` and `/help`: usage and Group Privacy Mode explanation.
- Known commands in private chats return only the explanation.
- Commands bypass probability and cooldown.
- Automatic replies apply only to ordinary group and supergroup text messages.

Use plain Telegram text. Never enable a parse mode for user-provided display names.

## Required checks

Run these before handing off changes:

```sh
make fmt-check
make vet
make test-race
make build
```

Run `docker build -t random-reaction-telegram-bot .` when Docker or build files change and the environment supports it.

## Secrets

Never commit, print, or log bot tokens, webhook secrets, real public service URLs, `.env` contents, or secret-manager output. Keep examples obviously fake. Polling deletes the token's existing webhook, so use a separate development token when testing manually.

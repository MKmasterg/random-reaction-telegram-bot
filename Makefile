BINARY := bin/random-reaction-telegram-bot

.PHONY: build run register-webhook redis-up fmt fmt-check vet test test-integration test-race check docker-build

build:
	mkdir -p bin
	go build -trimpath -o $(BINARY) ./cmd/bot

run:
	go run ./cmd/bot

register-webhook:
	go run ./cmd/register-webhook

redis-up:
	docker compose up -d redis

fmt:
	gofmt -w api cmd functions internal

fmt-check:
	test -z "$$(gofmt -l api cmd functions internal)"

vet:
	go vet ./...

test:
	go test ./...

test-integration:
	REDIS_TEST_URL=redis://localhost:6379 go test -run TestRedisCooldownIntegration ./internal/redisstore

test-race:
	go test -race ./...

check: fmt-check vet test-race

docker-build:
	docker build -t random-reaction-telegram-bot .

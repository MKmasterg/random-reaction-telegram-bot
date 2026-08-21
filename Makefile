BINARY := bin/random-reaction-telegram-bot

.PHONY: build run fmt fmt-check vet test test-race check docker-build

build:
	mkdir -p bin
	go build -trimpath -o $(BINARY) ./cmd/bot

run:
	go run ./cmd/bot

fmt:
	gofmt -w cmd internal

fmt-check:
	test -z "$$(gofmt -l cmd internal)"

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

check: fmt-check vet test-race

docker-build:
	docker build -t random-reaction-telegram-bot .

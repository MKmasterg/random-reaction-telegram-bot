FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/random-reaction-telegram-bot ./cmd/bot

FROM alpine:3.23

RUN apk add --no-cache ca-certificates \
    && addgroup -S bot \
    && adduser -S -G bot bot
COPY --from=build /out/random-reaction-telegram-bot /usr/local/bin/random-reaction-telegram-bot

USER bot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/random-reaction-telegram-bot"]

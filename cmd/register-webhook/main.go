package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	tgbot "github.com/go-telegram/bot"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/redisstore"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(); err != nil {
		logger.Error("register webhook", "error", err)
		os.Exit(1)
	}
	logger.Info("webhook registered")
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if cfg.Transport != config.TransportWebhook {
		return fmt.Errorf("BOT_TRANSPORT must be %q", config.TransportWebhook)
	}
	redisClient, err := redisstore.NewClient(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis readiness check: %w", err)
	}
	client, err := tgbot.New(cfg.Token, tgbot.WithSkipGetMe())
	if err != nil {
		return fmt.Errorf("initialize Telegram client: %w", err)
	}
	registrationCtx, registrationCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer registrationCancel()
	return registerReadyWebhook(registrationCtx, http.DefaultClient, client, cfg)
}

type httpDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type webhookSetter interface {
	SetWebhook(ctx context.Context, params *tgbot.SetWebhookParams) (bool, error)
}

func checkPublicHealth(ctx context.Context, client httpDoer, publicBaseURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, publicBaseURL+"/api/healthz", nil)
	if err != nil {
		return fmt.Errorf("create public readiness request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("check public readiness: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("public readiness returned HTTP %d", response.StatusCode)
	}
	return nil
}

func registerReadyWebhook(ctx context.Context, healthClient httpDoer, telegramClient webhookSetter, cfg config.Config) error {
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := checkPublicHealth(healthCtx, healthClient, cfg.PublicBaseURL)
	cancel()
	if err != nil {
		return err
	}
	return setWebhook(ctx, telegramClient, cfg)
}

func setWebhook(ctx context.Context, client webhookSetter, cfg config.Config) error {
	ok, err := client.SetWebhook(ctx, &tgbot.SetWebhookParams{
		URL:            cfg.WebhookURL(),
		AllowedUpdates: []string{"message"},
		SecretToken:    cfg.WebhookSecret,
	})
	if err != nil {
		return fmt.Errorf("set Telegram webhook: %w", err)
	}
	if !ok {
		return fmt.Errorf("Telegram rejected webhook registration")
	}
	return nil
}

package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/content"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/handler"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/reaction"
	telegramadapter "github.com/MKmasterg/random-reaction-telegram-bot/internal/telegram"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/transport"
)

type productionRandom struct{}

func (productionRandom) IntN(n int) int   { return rand.IntN(n) }
func (productionRandom) Float64() float64 { return rand.Float64() }

type appDispatcher struct {
	handler *handler.Handler
}

func (d appDispatcher) Dispatch(ctx context.Context, update *models.Update) error {
	return d.handler.Handle(ctx, telegramadapter.FromUpdate(update))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	contentModel, err := content.LoadEmbedded()
	if err != nil {
		return fmt.Errorf("load embedded content: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var application *handler.Handler
	options := []tgbot.Option{
		tgbot.WithDefaultHandler(func(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
			_ = application.Handle(ctx, telegramadapter.FromUpdate(update))
		}),
	}
	client, err := tgbot.New(cfg.Token, options...)
	if err != nil {
		return fmt.Errorf("initialize Telegram client: %w", err)
	}

	random := productionRandom{}
	generator := reaction.New(contentModel, random)
	cooldowns := handler.NewCooldowns(cfg.GroupCooldown)
	application = handler.New(telegramadapter.NewDelivery(client), generator, cooldowns, handler.Options{
		Probability: cfg.ReplyProbability,
		Random:      random,
		Now:         time.Now,
		Logger:      logger,
	})
	go cooldowns.RunCleanup(ctx, time.Now)

	switch cfg.Transport {
	case config.TransportPolling:
		return transport.RunPolling(ctx, client, logger)
	case config.TransportWebhook:
		httpHandler := transport.NewHTTPHandler(cfg.WebhookSecret, appDispatcher{handler: application}, logger)
		address := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
		return transport.RunWebhook(ctx, client, httpHandler, address, cfg.WebhookURL(), cfg.WebhookSecret, logger)
	default:
		return fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
}

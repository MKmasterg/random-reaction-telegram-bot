package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/application"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/handler"
	telegramadapter "github.com/MKmasterg/random-reaction-telegram-bot/internal/telegram"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/transport"
)

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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var runtime *application.Runtime
	options := []tgbot.Option{
		tgbot.WithDefaultHandler(func(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
			_ = runtime.Handler.Handle(ctx, telegramadapter.FromUpdate(update))
		}),
		tgbot.WithNotAsyncHandlers(),
		tgbot.WithErrorsHandler(func(err error) {
			logger.Error("telegram sdk", "error", err)
		}),
	}
	runtime, err = application.New(cfg, logger, options...)
	if err != nil {
		return err
	}
	defer runtime.Close()

	switch cfg.Transport {
	case config.TransportPolling:
		return transport.RunPolling(ctx, runtime.Bot, logger)
	case config.TransportWebhook:
		readyCtx, readyCancel := context.WithTimeout(ctx, time.Second)
		readyErr := runtime.Ready(readyCtx)
		readyCancel()
		if readyErr != nil {
			return fmt.Errorf("Redis readiness check: %w", readyErr)
		}
		httpHandler := transport.NewHTTPHandlerWithReadiness(cfg.WebhookSecret, appDispatcher{handler: runtime.Handler}, runtime.Ready, logger)
		address := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
		return transport.RunWebhook(ctx, runtime.Bot, httpHandler, address, cfg.WebhookURL(), cfg.WebhookSecret, logger)
	default:
		return fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
}

package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	tgbot "github.com/go-telegram/bot"
)

type pollingBot interface {
	DeleteWebhook(ctx context.Context, params *tgbot.DeleteWebhookParams) (bool, error)
	Start(ctx context.Context)
}

type webhookBot interface {
	SetWebhook(ctx context.Context, params *tgbot.SetWebhookParams) (bool, error)
}

// RunPolling removes any existing webhook before entering long polling.
func RunPolling(ctx context.Context, bot pollingBot, logger *slog.Logger) error {
	if _, err := bot.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{}); err != nil {
		return fmt.Errorf("delete existing webhook: %w", err)
	}
	logger.Info("starting bot transport", "mode", "polling")
	bot.Start(ctx)
	return nil
}

// RunWebhook registers the public endpoint and serves until ctx is canceled.
func RunWebhook(ctx context.Context, bot webhookBot, handler http.Handler, address, publicURL, secret string, logger *slog.Logger) error {
	return runWebhook(ctx, bot, handler, address, publicURL, secret, logger, net.Listen)
}

func runWebhook(ctx context.Context, bot webhookBot, handler http.Handler, address, publicURL, secret string, logger *slog.Logger, listen func(network, address string) (net.Listener, error)) error {
	listener, err := listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for webhook: %w", err)
	}
	if _, err := bot.SetWebhook(ctx, &tgbot.SetWebhookParams{
		URL:            publicURL,
		AllowedUpdates: []string{"message"},
		SecretToken:    secret,
	}); err != nil {
		_ = listener.Close()
		return fmt.Errorf("register webhook: %w", err)
	}

	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting bot transport", "mode", "webhook", "address", address)
		errCh <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down webhook server: %w", err)
		}
		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve webhook: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve webhook: %w", err)
		}
		return nil
	}
}

package functions

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/application"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/redisstore"
)

type Lookup func(string) (string, bool)

func NewWebhookHandler(lookup Lookup, logger *slog.Logger) (http.Handler, error) {
	cfg, err := config.FromFunctionLookup(lookup)
	if err != nil {
		return nil, err
	}
	return application.NewFunctionHandler(cfg, logger)
}

func NewHealthHandler(lookup Lookup) (http.Handler, error) {
	cfg, err := config.FromFunctionLookup(lookup)
	if err != nil {
		return nil, err
	}
	client, err := redisstore.NewClient(cfg.RedisURL)
	if err != nil {
		return nil, err
	}
	return &healthHandler{client: client}, nil
}

type healthHandler struct {
	client *redis.Client
}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.client.Ping(ctx).Err(); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

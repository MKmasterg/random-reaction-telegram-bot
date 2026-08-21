package handler

import (
	"context"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/redisstore"
)

var (
	initOnce sync.Once
	client   *redis.Client
	initErr  error
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	initOnce.Do(func() {
		var cfg config.Config
		cfg, initErr = config.FromFunctionLookup(os.LookupEnv)
		if initErr == nil {
			client, initErr = redisstore.NewClient(cfg.RedisURL)
		}
	})
	if initErr != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

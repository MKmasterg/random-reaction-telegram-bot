package handler

import (
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/MKmasterg/random-reaction-telegram-bot/functions"
)

var (
	initOnce sync.Once
	endpoint http.Handler
	initErr  error
	logger   = slog.New(slog.NewJSONHandler(os.Stdout, nil))
)

func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(func() {
		endpoint, initErr = functions.NewWebhookHandler(os.LookupEnv, logger)
	})
	if initErr != nil {
		logger.Error("initialize webhook function", "error", initErr)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	endpoint.ServeHTTP(w, r)
}

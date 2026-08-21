package handler

import (
	"net/http"
	"os"
	"sync"

	"github.com/MKmasterg/random-reaction-telegram-bot/functions"
)

var (
	initOnce sync.Once
	endpoint http.Handler
	initErr  error
)

func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(func() {
		endpoint, initErr = functions.NewHealthHandler(os.LookupEnv)
	})
	if initErr != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	endpoint.ServeHTTP(w, r)
}

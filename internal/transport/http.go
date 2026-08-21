package transport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
)

const maxWebhookBodyBytes = 1 << 20
const dedupeCapacity = 4096

type Dispatcher interface {
	Dispatch(ctx context.Context, update *models.Update) error
}

// NewHTTPHandler exposes the standalone server routes.
func NewHTTPHandler(secret string, dispatcher Dispatcher, logger *slog.Logger) http.Handler {
	return NewHTTPHandlerWithReadiness(secret, dispatcher, nil, logger)
}

func NewHTTPHandlerWithReadiness(secret string, dispatcher Dispatcher, readiness func(context.Context) error, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	health := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if readiness != nil {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := readiness(ctx); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	}
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/api/healthz", health)
	webhook := NewWebhookHandler(secret, dispatcher, logger)
	mux.Handle("/api/webhook", webhook)
	mux.Handle("/telegram/webhook", webhook)
	return mux
}

func NewWebhookHandler(secret string, dispatcher Dispatcher, logger *slog.Logger) http.Handler {
	deduper := newUpdateDeduper(dedupeCapacity)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provided := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if !secretsEqual(provided, secret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		body := http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)
		defer body.Close()
		decoder := json.NewDecoder(body)
		var update models.Update
		if err := decoder.Decode(&update); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			logger.Warn("reject malformed webhook", "error", err)
			http.Error(w, "malformed update", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			logger.Warn("reject webhook with trailing data")
			http.Error(w, "malformed update", http.StatusBadRequest)
			return
		}
		if update.ID <= 0 {
			http.Error(w, "malformed update", http.StatusBadRequest)
			return
		}
		switch deduper.Begin(update.ID) {
		case updateCompleted:
			logger.Info("ignore duplicate webhook update", "update_id", update.ID)
			w.WriteHeader(http.StatusOK)
			return
		case updateInFlight:
			logger.Info("defer concurrent webhook update", "update_id", update.ID)
			http.Error(w, "update already in progress", http.StatusServiceUnavailable)
			return
		}
		completed := false
		defer func() {
			if !completed {
				deduper.Abort(update.ID)
			}
		}()
		if err := dispatcher.Dispatch(r.Context(), &update); err != nil {
			logger.Error("dispatch webhook update", "update_id", update.ID, "error", err)
			http.Error(w, "update processing failed", http.StatusServiceUnavailable)
			return
		}
		deduper.Complete(update.ID)
		completed = true
		w.WriteHeader(http.StatusOK)
	})
}

type updateDeduper struct {
	mu        sync.Mutex
	capacity  int
	completed map[int64]struct{}
	inFlight  map[int64]struct{}
	order     []int64
	next      int
}

func newUpdateDeduper(capacity int) *updateDeduper {
	return &updateDeduper{
		capacity:  capacity,
		completed: make(map[int64]struct{}),
		inFlight:  make(map[int64]struct{}),
		order:     make([]int64, 0, capacity),
	}
}

type updateState uint8

const (
	updateNew updateState = iota
	updateInFlight
	updateCompleted
)

// Begin reserves an update ID or reports its current processing state.
func (d *updateDeduper) Begin(updateID int64) updateState {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.completed[updateID]; exists {
		return updateCompleted
	}
	if _, exists := d.inFlight[updateID]; exists {
		return updateInFlight
	}
	d.inFlight[updateID] = struct{}{}
	return updateNew
}

// Complete commits a successfully processed update to the bounded history.
func (d *updateDeduper) Complete(updateID int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.inFlight, updateID)
	if _, exists := d.completed[updateID]; exists {
		return
	}
	if len(d.order) < d.capacity {
		d.order = append(d.order, updateID)
	} else {
		delete(d.completed, d.order[d.next])
		d.order[d.next] = updateID
		d.next = (d.next + 1) % d.capacity
	}
	d.completed[updateID] = struct{}{}
}

// Abort releases a failed reservation so Telegram can retry the update.
func (d *updateDeduper) Abort(updateID int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.inFlight, updateID)
}

func secretsEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

package functions

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFunctionHandlersInitializeWithoutNetwork(t *testing.T) {
	lookup := testLookup(map[string]string{
		"TELEGRAM_BOT_TOKEN": "test-token",
		"WEBHOOK_SECRET":     "test-secret",
		"REDIS_URL":          "redis://localhost:6379",
		"REDIS_KEY_PREFIX":   "random-reaction:test",
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := NewWebhookHandler(lookup, logger); err != nil {
		t.Fatalf("NewWebhookHandler() error = %v", err)
	}
	health, err := NewHealthHandler(lookup)
	if err != nil {
		t.Fatalf("NewHealthHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/healthz", nil)
	recorder := httptest.NewRecorder()
	health.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST health status = %d", recorder.Code)
	}
}

func TestFunctionHandlersRejectInvalidConfig(t *testing.T) {
	lookup := testLookup(map[string]string{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := NewWebhookHandler(lookup, logger); err == nil {
		t.Fatal("NewWebhookHandler() error = nil")
	}
	if _, err := NewHealthHandler(lookup); err == nil {
		t.Fatal("NewHealthHandler() error = nil")
	}
}

func testLookup(values map[string]string) Lookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

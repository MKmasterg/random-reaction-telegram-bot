package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot/models"
)

type recordingDispatcher struct {
	mu      sync.Mutex
	updates []*models.Update
	err     error
}

func (d *recordingDispatcher) Dispatch(_ context.Context, update *models.Update) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.updates = append(d.updates, update)
	return d.err
}

func TestHealthHandler(t *testing.T) {
	handler := testHTTPHandler(&recordingDispatcher{})

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok\n" {
		t.Fatalf("GET /healthz = %d %q", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/healthz", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST /healthz = %d Allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestWebhookRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		secret     string
		wantStatus int
	}{
		{name: "method", method: http.MethodGet, secret: "test-secret", wantStatus: http.StatusMethodNotAllowed},
		{name: "missing secret", method: http.MethodPost, body: `{}`, wantStatus: http.StatusUnauthorized},
		{name: "wrong secret", method: http.MethodPost, body: `{}`, secret: "wrong", wantStatus: http.StatusUnauthorized},
		{name: "malformed", method: http.MethodPost, body: `{`, secret: "test-secret", wantStatus: http.StatusBadRequest},
		{name: "trailing JSON", method: http.MethodPost, body: `{} {}`, secret: "test-secret", wantStatus: http.StatusBadRequest},
		{name: "missing update ID", method: http.MethodPost, body: `{}`, secret: "test-secret", wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatcher := &recordingDispatcher{}
			handler := testHTTPHandler(dispatcher)
			request := httptest.NewRequest(test.method, "/telegram/webhook", strings.NewReader(test.body))
			if test.secret != "" {
				request.Header.Set("X-Telegram-Bot-Api-Secret-Token", test.secret)
			}
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d; body=%q", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if len(dispatcher.updates) != 0 {
				t.Errorf("dispatched %d malformed updates", len(dispatcher.updates))
			}
		})
	}
}

func TestWebhookRejectsOversizedBody(t *testing.T) {
	dispatcher := &recordingDispatcher{}
	handler := testHTTPHandler(dispatcher)
	body := `{"update_id":1,"padding":"` + strings.Repeat("x", maxWebhookBodyBytes) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if len(dispatcher.updates) != 0 {
		t.Fatalf("dispatched %d oversized updates", len(dispatcher.updates))
	}
}

func TestWebhookDispatchesValidUpdate(t *testing.T) {
	dispatcher := &recordingDispatcher{}
	handler := testHTTPHandler(dispatcher)
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{"update_id":42,"message":{"message_id":7,"date":1,"chat":{"id":9,"type":"group"},"text":"hello"}}`))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", recorder.Code, recorder.Body.String())
	}
	if len(dispatcher.updates) != 1 || dispatcher.updates[0].ID != 42 {
		t.Fatalf("updates = %+v, want update 42", dispatcher.updates)
	}
}

func TestWebhookDeduplicatesUpdateID(t *testing.T) {
	dispatcher := &recordingDispatcher{}
	handler := testHTTPHandler(dispatcher)
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{"update_id":42}`))
		request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
	}
	if len(dispatcher.updates) != 1 {
		t.Fatalf("dispatched updates = %d, want 1", len(dispatcher.updates))
	}
}

func TestWebhookFailureIsRetryable(t *testing.T) {
	dispatcher := &recordingDispatcher{err: errors.New("temporary failure")}
	handler := testHTTPHandler(dispatcher)

	first := performWebhookRequest(handler, `{"update_id":42}`)
	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d, want 503", first.Code)
	}

	dispatcher.mu.Lock()
	dispatcher.err = nil
	dispatcher.mu.Unlock()
	second := performWebhookRequest(handler, `{"update_id":42}`)
	if second.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", second.Code)
	}

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.updates) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(dispatcher.updates))
	}
}

func TestWebhookConcurrentDuplicateIsRetryable(t *testing.T) {
	dispatcher := &blockingDispatcher{started: make(chan struct{}), release: make(chan struct{})}
	handler := testHTTPHandler(dispatcher)
	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstResult <- performWebhookRequest(handler, `{"update_id":42}`)
	}()
	<-dispatcher.started

	concurrent := performWebhookRequest(handler, `{"update_id":42}`)
	if concurrent.Code != http.StatusServiceUnavailable {
		t.Fatalf("concurrent status = %d, want 503", concurrent.Code)
	}
	close(dispatcher.release)
	if first := <-firstResult; first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	duplicate := performWebhookRequest(handler, `{"update_id":42}`)
	if duplicate.Code != http.StatusOK {
		t.Fatalf("completed duplicate status = %d, want 200", duplicate.Code)
	}
	if dispatcher.dispatches != 1 {
		t.Fatalf("dispatch count = %d, want 1", dispatcher.dispatches)
	}
}

type blockingDispatcher struct {
	started    chan struct{}
	release    chan struct{}
	dispatches int
}

func (d *blockingDispatcher) Dispatch(_ context.Context, _ *models.Update) error {
	d.dispatches++
	close(d.started)
	<-d.release
	return nil
}

func TestUpdateDeduperEvictsOldestID(t *testing.T) {
	deduper := newUpdateDeduper(2)
	if deduper.Begin(1) != updateNew {
		t.Fatal("new ID 1 was not reserved")
	}
	deduper.Complete(1)
	if deduper.Begin(2) != updateNew {
		t.Fatal("new ID 2 was not reserved")
	}
	deduper.Complete(2)
	if deduper.Begin(1) != updateCompleted {
		t.Fatal("retained ID not reported as completed")
	}
	if deduper.Begin(3) != updateNew {
		t.Fatal("new ID 3 was not reserved")
	}
	deduper.Complete(3)
	if deduper.Begin(1) != updateNew {
		t.Fatal("evicted ID was not reusable")
	}
	deduper.Abort(1)
	if deduper.Begin(1) != updateNew {
		t.Fatal("aborted ID was not reusable")
	}
}

func TestUnknownPath(t *testing.T) {
	handler := testHTTPHandler(&recordingDispatcher{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func testHTTPHandler(dispatcher Dispatcher) http.Handler {
	return NewHTTPHandler("test-secret", dispatcher, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func performWebhookRequest(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

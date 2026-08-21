package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	tgbot "github.com/go-telegram/bot"
)

type fakePollingBot struct {
	deleted bool
	started bool
	reject  bool
	err     error
}

func (b *fakePollingBot) DeleteWebhook(_ context.Context, _ *tgbot.DeleteWebhookParams) (bool, error) {
	b.deleted = true
	return !b.reject && b.err == nil, b.err
}

func (b *fakePollingBot) Start(_ context.Context) {
	if !b.deleted {
		panic("polling started before webhook deletion")
	}
	b.started = true
}

func TestRunPollingDeletesWebhookBeforeStart(t *testing.T) {
	client := &fakePollingBot{}
	if err := RunPolling(context.Background(), client, discardLogger()); err != nil {
		t.Fatalf("RunPolling() error = %v", err)
	}
	if !client.deleted || !client.started {
		t.Fatalf("client = %+v, want deleted and started", client)
	}
}

func TestRunPollingStopsWhenWebhookDeletionFails(t *testing.T) {
	client := &fakePollingBot{err: errors.New("delete failed")}
	if err := RunPolling(context.Background(), client, discardLogger()); err == nil {
		t.Fatal("RunPolling() error = nil")
	}
	if client.started {
		t.Fatal("polling started after deletion failure")
	}
}

func TestRunPollingStopsWhenWebhookDeletionIsRejected(t *testing.T) {
	client := &fakePollingBot{reject: true}
	if err := RunPolling(context.Background(), client, discardLogger()); err == nil {
		t.Fatal("RunPolling() error = nil")
	}
	if client.started {
		t.Fatal("polling started after deletion rejection")
	}
}

type failingWebhookBot struct {
	params *tgbot.SetWebhookParams
	err    error
}

type fakeListener struct {
	closed bool
}

func (l *fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not accepting") }
func (l *fakeListener) Close() error {
	l.closed = true
	return nil
}
func (l *fakeListener) Addr() net.Addr { return fakeAddress("127.0.0.1:1234") }

type fakeAddress string

func (a fakeAddress) Network() string { return "tcp" }
func (a fakeAddress) String() string  { return string(a) }

func (b *failingWebhookBot) SetWebhook(_ context.Context, params *tgbot.SetWebhookParams) (bool, error) {
	b.params = params
	return false, b.err
}

func TestRunWebhookRegistrationParameters(t *testing.T) {
	client := &failingWebhookBot{err: errors.New("registration failed")}
	listener := &fakeListener{}
	err := runWebhook(
		context.Background(),
		client,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		"127.0.0.1:0",
		"https://bot.example.test/telegram/webhook",
		"secret-value",
		discardLogger(),
		func(_, _ string) (net.Listener, error) { return listener, nil },
	)
	if err == nil {
		t.Fatal("RunWebhook() error = nil")
	}
	if client.params == nil {
		t.Fatal("SetWebhook() was not called")
	}
	if client.params.URL != "https://bot.example.test/telegram/webhook" || client.params.SecretToken != "secret-value" {
		t.Errorf("SetWebhook params = %+v", client.params)
	}
	if len(client.params.AllowedUpdates) != 1 || client.params.AllowedUpdates[0] != "message" {
		t.Errorf("AllowedUpdates = %v, want [message]", client.params.AllowedUpdates)
	}
	if !listener.closed {
		t.Error("listener was not closed after registration failure")
	}
}

func TestRunWebhookStopsWhenRegistrationIsRejected(t *testing.T) {
	client := &failingWebhookBot{}
	listener := &fakeListener{}
	err := runWebhook(
		context.Background(), client, http.NotFoundHandler(), "127.0.0.1:0",
		"https://bot.example.test/api/webhook", "secret", discardLogger(),
		func(_, _ string) (net.Listener, error) { return listener, nil },
	)
	if err == nil || !listener.closed {
		t.Fatalf("runWebhook() error = %v, listener closed = %v", err, listener.closed)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

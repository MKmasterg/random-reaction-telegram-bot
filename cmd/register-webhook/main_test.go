package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
)

type fakeHTTPClient struct {
	response *http.Response
	err      error
	request  *http.Request
}

func (c *fakeHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c.request = request
	return c.response, c.err
}

type fakeWebhookSetter struct {
	called bool
	params *tgbot.SetWebhookParams
	ok     bool
	err    error
}

func (s *fakeWebhookSetter) SetWebhook(_ context.Context, params *tgbot.SetWebhookParams) (bool, error) {
	s.called = true
	s.params = params
	return s.ok, s.err
}

func TestCheckPublicHealth(t *testing.T) {
	tests := []struct {
		name    string
		client  *fakeHTTPClient
		wantErr bool
	}{
		{
			name: "ready",
			client: &fakeHTTPClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok\n")),
			}},
		},
		{
			name: "not ready",
			client: &fakeHTTPClient{response: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("not ready\n")),
			}},
			wantErr: true,
		},
		{
			name:    "request failure",
			client:  &fakeHTTPClient{err: errors.New("offline")},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkPublicHealth(context.Background(), test.client, "https://bot.example.test")
			if (err != nil) != test.wantErr {
				t.Fatalf("checkPublicHealth() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.client.request != nil && test.client.request.URL.String() != "https://bot.example.test/api/healthz" {
				t.Errorf("request URL = %q", test.client.request.URL.String())
			}
		})
	}
}

func TestReadinessFailureDoesNotSetWebhook(t *testing.T) {
	health := &fakeHTTPClient{response: &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       io.NopCloser(strings.NewReader("not ready\n")),
	}}
	setter := &fakeWebhookSetter{ok: true}
	cfg := config.Config{PublicBaseURL: "https://bot.example.test", WebhookSecret: "secret"}
	if err := registerReadyWebhook(context.Background(), health, setter, cfg); err == nil {
		t.Fatal("registerReadyWebhook() error = nil")
	}
	if setter.called {
		t.Fatal("SetWebhook() was called after readiness failure")
	}
}

func TestSetWebhook(t *testing.T) {
	cfg := config.Config{PublicBaseURL: "https://bot.example.test", WebhookSecret: "secret"}
	setter := &fakeWebhookSetter{ok: true}
	if err := setWebhook(context.Background(), setter, cfg); err != nil {
		t.Fatalf("setWebhook() error = %v", err)
	}
	if !setter.called || setter.params.URL != "https://bot.example.test/api/webhook" {
		t.Fatalf("SetWebhook() params = %+v", setter.params)
	}

	setter = &fakeWebhookSetter{ok: false}
	if err := setWebhook(context.Background(), setter, cfg); err == nil {
		t.Fatal("setWebhook() error = nil for Telegram rejection")
	}
}

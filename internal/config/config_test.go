package config

import (
	"strings"
	"testing"
	"time"
)

func TestFromLookupDefaults(t *testing.T) {
	cfg, err := FromLookup(mapLookup(map[string]string{"TELEGRAM_BOT_TOKEN": "test-token"}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if cfg.Token != "test-token" || cfg.Transport != TransportPolling {
		t.Fatalf("unexpected basic config: %+v", cfg)
	}
	if cfg.ReplyProbability != 0.02 {
		t.Errorf("ReplyProbability = %v, want 0.02", cfg.ReplyProbability)
	}
	if cfg.GroupCooldown != 5*time.Minute {
		t.Errorf("GroupCooldown = %v, want 5m", cfg.GroupCooldown)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
}

func TestFromLookupWebhook(t *testing.T) {
	cfg, err := FromLookup(mapLookup(map[string]string{
		"TELEGRAM_BOT_TOKEN": "token",
		"BOT_TRANSPORT":      "WEBHOOK",
		"REPLY_PROBABILITY":  "0.5",
		"GROUP_COOLDOWN":     "30s",
		"PORT":               "9090",
		"PUBLIC_BASE_URL":    "https://bot.example.test/",
		"WEBHOOK_SECRET":     "secret",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if cfg.Transport != TransportWebhook || cfg.WebhookURL() != "https://bot.example.test/telegram/webhook" {
		t.Fatalf("unexpected webhook config: %+v", cfg)
	}
	if cfg.ReplyProbability != 0.5 || cfg.GroupCooldown != 30*time.Second || cfg.Port != 9090 {
		t.Fatalf("custom values were not applied: %+v", cfg)
	}
}

func TestFromLookupErrors(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		wantErr string
	}{
		{name: "missing token", values: map[string]string{}, wantErr: "TELEGRAM_BOT_TOKEN"},
		{name: "transport", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "BOT_TRANSPORT": "carrier-pigeon"}, wantErr: "BOT_TRANSPORT"},
		{name: "probability syntax", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "REPLY_PROBABILITY": "often"}, wantErr: "REPLY_PROBABILITY"},
		{name: "zero probability", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "REPLY_PROBABILITY": "0"}, wantErr: "REPLY_PROBABILITY"},
		{name: "NaN probability", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "REPLY_PROBABILITY": "NaN"}, wantErr: "REPLY_PROBABILITY"},
		{name: "high probability", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "REPLY_PROBABILITY": "1.01"}, wantErr: "REPLY_PROBABILITY"},
		{name: "duration syntax", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "GROUP_COOLDOWN": "later"}, wantErr: "GROUP_COOLDOWN"},
		{name: "negative duration", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "GROUP_COOLDOWN": "-1s"}, wantErr: "GROUP_COOLDOWN"},
		{name: "port syntax", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "PORT": "http"}, wantErr: "PORT"},
		{name: "port range", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "PORT": "70000"}, wantErr: "PORT"},
		{name: "webhook missing URL", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "BOT_TRANSPORT": "webhook", "WEBHOOK_SECRET": "x"}, wantErr: "PUBLIC_BASE_URL"},
		{name: "webhook non-HTTPS URL", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "BOT_TRANSPORT": "webhook", "PUBLIC_BASE_URL": "http://example.test", "WEBHOOK_SECRET": "x"}, wantErr: "HTTPS"},
		{name: "webhook URL with query", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "BOT_TRANSPORT": "webhook", "PUBLIC_BASE_URL": "https://example.test?token=x", "WEBHOOK_SECRET": "x"}, wantErr: "HTTPS"},
		{name: "webhook missing secret", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "BOT_TRANSPORT": "webhook", "PUBLIC_BASE_URL": "https://example.test"}, wantErr: "WEBHOOK_SECRET"},
		{name: "webhook invalid secret", values: map[string]string{"TELEGRAM_BOT_TOKEN": "x", "BOT_TRANSPORT": "webhook", "PUBLIC_BASE_URL": "https://example.test", "WEBHOOK_SECRET": "has spaces"}, wantErr: "WEBHOOK_SECRET"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FromLookup(mapLookup(test.values))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("FromLookup() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

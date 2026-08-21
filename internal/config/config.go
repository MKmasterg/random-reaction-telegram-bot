package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TransportPolling = "polling"
	TransportWebhook = "webhook"
)

// Config is the complete environment-based runtime configuration.
type Config struct {
	Token            string
	Transport        string
	ReplyProbability float64
	GroupCooldown    time.Duration
	Port             int
	PublicBaseURL    string
	WebhookSecret    string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup reads configuration through lookup, which keeps tests independent
// of the process environment.
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{
		Transport:        TransportPolling,
		ReplyProbability: 0.02,
		GroupCooldown:    5 * time.Minute,
		Port:             8080,
	}

	cfg.Token = value(lookup, "TELEGRAM_BOT_TOKEN")
	if cfg.Token == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	if raw := value(lookup, "BOT_TRANSPORT"); raw != "" {
		cfg.Transport = strings.ToLower(raw)
	}
	if cfg.Transport != TransportPolling && cfg.Transport != TransportWebhook {
		return Config{}, fmt.Errorf("BOT_TRANSPORT must be %q or %q", TransportPolling, TransportWebhook)
	}

	if raw := value(lookup, "REPLY_PROBABILITY"); raw != "" {
		probability, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse REPLY_PROBABILITY: %w", err)
		}
		cfg.ReplyProbability = probability
	}
	if math.IsNaN(cfg.ReplyProbability) || math.IsInf(cfg.ReplyProbability, 0) || cfg.ReplyProbability <= 0 || cfg.ReplyProbability > 1 {
		return Config{}, errors.New("REPLY_PROBABILITY must be greater than 0 and at most 1")
	}

	if raw := value(lookup, "GROUP_COOLDOWN"); raw != "" {
		cooldown, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse GROUP_COOLDOWN: %w", err)
		}
		cfg.GroupCooldown = cooldown
	}
	if cfg.GroupCooldown <= 0 {
		return Config{}, errors.New("GROUP_COOLDOWN must be greater than 0")
	}

	if raw := value(lookup, "PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PORT: %w", err)
		}
		cfg.Port = port
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, errors.New("PORT must be between 1 and 65535")
	}

	cfg.PublicBaseURL = strings.TrimRight(value(lookup, "PUBLIC_BASE_URL"), "/")
	cfg.WebhookSecret = value(lookup, "WEBHOOK_SECRET")
	if cfg.Transport == TransportWebhook {
		if cfg.PublicBaseURL == "" {
			return Config{}, errors.New("PUBLIC_BASE_URL is required in webhook mode")
		}
		parsed, err := url.Parse(cfg.PublicBaseURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, errors.New("PUBLIC_BASE_URL must be an absolute HTTPS URL")
		}
		if cfg.WebhookSecret == "" {
			return Config{}, errors.New("WEBHOOK_SECRET is required in webhook mode")
		}
		if !validWebhookSecret(cfg.WebhookSecret) {
			return Config{}, errors.New("WEBHOOK_SECRET must be 1-256 characters using only A-Z, a-z, 0-9, underscore, and hyphen")
		}
	}

	return cfg, nil
}

// WebhookURL returns the public URL registered with Telegram.
func (c Config) WebhookURL() string {
	return c.PublicBaseURL + "/telegram/webhook"
}

func value(lookup func(string) (string, bool), key string) string {
	raw, _ := lookup(key)
	return strings.TrimSpace(raw)
}

func validWebhookSecret(secret string) bool {
	if len(secret) < 1 || len(secret) > 256 {
		return false
	}
	for _, char := range secret {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

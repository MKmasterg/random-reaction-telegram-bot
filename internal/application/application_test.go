package application

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
)

func TestNewBuildsSharedRuntimeWithoutNetwork(t *testing.T) {
	runtime, err := New(config.Config{
		Token:            "test-token",
		ReplyProbability: 0.02,
		GroupCooldown:    5 * time.Minute,
		RedisURL:         "redis://localhost:6379",
		RedisKeyPrefix:   "random-reaction:test",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer runtime.Close()
	if runtime.Bot == nil || runtime.Handler == nil || runtime.Redis == nil {
		t.Fatalf("runtime = %+v", runtime)
	}
}

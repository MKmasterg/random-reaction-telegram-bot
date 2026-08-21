package redisstore

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeExecutor struct {
	states []int64
	errors []error
	calls  []scriptCall
}

type scriptCall struct {
	script *redis.Script
	keys   []string
	args   []any
}

func (f *fakeExecutor) Run(_ context.Context, script *redis.Script, keys []string, args ...any) (int64, error) {
	f.calls = append(f.calls, scriptCall{script: script, keys: append([]string(nil), keys...), args: append([]any(nil), args...)})
	index := len(f.calls) - 1
	if index < len(f.errors) && f.errors[index] != nil {
		return 0, f.errors[index]
	}
	return f.states[index], nil
}

func TestCooldownLifecycle(t *testing.T) {
	executor := &fakeExecutor{states: []int64{1, 1}}
	store := &Cooldowns{executor: executor, prefix: "bot:dev", duration: 5 * time.Minute}
	reservation, started, err := store.TryStart(context.Background(), 99, -42, time.Time{})
	if err != nil || !started {
		t.Fatalf("TryStart() = %v, %v", started, err)
	}
	if err := reservation.Finish(context.Background(), time.Time{}, true); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if executor.calls[0].keys[0] != "bot:dev:cooldown:-42" {
		t.Errorf("key = %q", executor.calls[0].keys[0])
	}
	if !strings.HasPrefix(executor.calls[0].args[1].(string), "pending:99:") {
		t.Errorf("pending value = %q", executor.calls[0].args[1])
	}
	if executor.calls[1].args[1] != 1 || executor.calls[1].args[2] != int64((5*time.Minute).Milliseconds()) {
		t.Errorf("finish args = %v", executor.calls[1].args)
	}
}

func TestCooldownStatesAndErrors(t *testing.T) {
	tests := []struct {
		name      string
		state     int64
		err       error
		wantStart bool
		wantErr   bool
	}{
		{name: "busy", state: 0},
		{name: "acquired", state: 1, wantStart: true},
		{name: "same update", state: 2, wantErr: true},
		{name: "unexpected", state: 3, wantErr: true},
		{name: "Redis error", err: errors.New("offline"), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := &fakeExecutor{states: []int64{test.state}, errors: []error{test.err}}
			store := &Cooldowns{executor: executor, prefix: "bot", duration: time.Minute}
			_, started, err := store.TryStart(context.Background(), 1, 2, time.Time{})
			if started != test.wantStart || (err != nil) != test.wantErr {
				t.Fatalf("TryStart() = %v, %v", started, err)
			}
		})
	}
}

func TestExpiredCommitFails(t *testing.T) {
	executor := &fakeExecutor{states: []int64{1, 0}}
	store := &Cooldowns{executor: executor, prefix: "bot", duration: time.Minute}
	reservation, _, _ := store.TryStart(context.Background(), 1, 2, time.Time{})
	if err := reservation.Finish(context.Background(), time.Time{}, true); err == nil {
		t.Fatal("Finish() error = nil")
	}
}

func TestRedisCooldownIntegration(t *testing.T) {
	rawURL := os.Getenv("REDIS_TEST_URL")
	if rawURL == "" {
		t.Skip("REDIS_TEST_URL is not set")
	}
	client, err := NewClient(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	prefix := "random-reaction:test:" + time.Now().Format("150405.000000000")
	storeA := NewCooldowns(client, prefix, 20*time.Millisecond)
	storeB := NewCooldowns(client, prefix, 20*time.Millisecond)
	isolatedStore := NewCooldowns(client, prefix+":isolated", 20*time.Millisecond)
	defer client.Del(ctx, storeA.key(42), isolatedStore.key(42))

	first, started, err := storeA.TryStart(ctx, 10, 42, time.Time{})
	if err != nil || !started {
		t.Fatalf("first reserve = %v, %v", started, err)
	}
	if _, started, err := storeB.TryStart(ctx, 10, 42, time.Time{}); err == nil || started {
		t.Fatalf("same-update reserve = %v, %v", started, err)
	}
	if _, started, err := storeB.TryStart(ctx, 11, 42, time.Time{}); err != nil || started {
		t.Fatalf("competing reserve = %v, %v", started, err)
	}
	isolated, started, err := isolatedStore.TryStart(ctx, 11, 42, time.Time{})
	if err != nil || !started {
		t.Fatalf("isolated-prefix reserve = %v, %v", started, err)
	}
	if err := isolated.Finish(ctx, time.Time{}, false); err != nil {
		t.Fatal(err)
	}
	if err := first.Finish(ctx, time.Time{}, true); err != nil {
		t.Fatal(err)
	}
	if _, started, err := storeB.TryStart(ctx, 12, 42, time.Time{}); err != nil || started {
		t.Fatalf("reserve during cooldown = %v, %v", started, err)
	}
	deadline := time.Now().Add(time.Second)
	var second *reservation
	for time.Now().Before(deadline) {
		candidate, acquired, reserveErr := storeB.TryStart(ctx, 13, 42, time.Time{})
		if reserveErr != nil {
			t.Fatal(reserveErr)
		}
		if acquired {
			second = candidate.(*reservation)
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if second == nil {
		t.Fatal("cooldown did not expire within one second")
	}
	if err := second.Finish(ctx, time.Time{}, false); err != nil {
		t.Fatal(err)
	}
	if _, started, err := storeA.TryStart(ctx, 14, 42, time.Time{}); err != nil || !started {
		t.Fatalf("reserve after release = %v, %v", started, err)
	}
}

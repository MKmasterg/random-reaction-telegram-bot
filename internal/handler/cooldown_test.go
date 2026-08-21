package handler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryCooldownEntry struct {
	lastSuccess time.Time
	inFlight    bool
}

type memoryCooldowns struct {
	mu        sync.Mutex
	duration  time.Duration
	retention time.Duration
	entries   map[int64]memoryCooldownEntry
}

func newMemoryCooldowns(duration time.Duration) *memoryCooldowns {
	return &memoryCooldowns{duration: duration, retention: 2 * duration, entries: make(map[int64]memoryCooldownEntry)}
}

func (c *memoryCooldowns) TryStart(_ context.Context, _, chatID int64, now time.Time) (CooldownReservation, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[chatID]
	if entry.inFlight || (exists && !entry.lastSuccess.IsZero() && now.Sub(entry.lastSuccess) < c.duration) {
		return nil, false, nil
	}
	entry.inFlight = true
	c.entries[chatID] = entry
	return &memoryReservation{store: c, chatID: chatID}, true, nil
}

type memoryReservation struct {
	store  *memoryCooldowns
	chatID int64
}

func (r *memoryReservation) Finish(_ context.Context, now time.Time, success bool) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	entry := r.store.entries[r.chatID]
	entry.inFlight = false
	if success {
		entry.lastSuccess = now
		r.store.entries[r.chatID] = entry
	} else if entry.lastSuccess.IsZero() {
		delete(r.store.entries, r.chatID)
	}
	return nil
}

func (c *memoryCooldowns) Cleanup(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for chatID, entry := range c.entries {
		if !entry.inFlight && !entry.lastSuccess.IsZero() && now.Sub(entry.lastSuccess) >= c.retention {
			delete(c.entries, chatID)
		}
	}
}

func TestCooldownReservationAndCleanup(t *testing.T) {
	store := newMemoryCooldowns(5 * time.Minute)
	ctx := context.Background()
	now := time.Unix(100, 0)
	reservation, started, err := store.TryStart(ctx, 101, 1, now)
	if err != nil || !started {
		t.Fatal("first TryStart() = false")
	}
	if _, started, _ := store.TryStart(ctx, 102, 1, now); started {
		t.Fatal("concurrent TryStart() = true")
	}
	_ = reservation.Finish(ctx, now, true)
	if _, started, _ := store.TryStart(ctx, 103, 1, now.Add(5*time.Minute-time.Nanosecond)); started {
		t.Fatal("TryStart() inside cooldown = true")
	}
	reservation, started, err = store.TryStart(ctx, 104, 1, now.Add(5*time.Minute))
	if err != nil || !started {
		t.Fatal("TryStart() at boundary = false")
	}
	_ = reservation.Finish(ctx, now.Add(5*time.Minute), true)
	store.Cleanup(now.Add(15 * time.Minute))
	if len(store.entries) != 0 {
		t.Fatalf("entries after cleanup = %d, want 0", len(store.entries))
	}
}

func TestFailedReservationIsRemoved(t *testing.T) {
	store := newMemoryCooldowns(time.Minute)
	ctx := context.Background()
	now := time.Unix(100, 0)
	reservation, started, err := store.TryStart(ctx, 101, 1, now)
	if err != nil || !started {
		t.Fatal("first TryStart() = false")
	}
	_ = reservation.Finish(ctx, now, false)
	if _, started, err := store.TryStart(ctx, 102, 1, now); err != nil || !started {
		t.Fatal("TryStart() after failure = false")
	}
}

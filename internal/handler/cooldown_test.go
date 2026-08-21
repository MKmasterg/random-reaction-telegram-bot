package handler

import (
	"testing"
	"time"
)

func TestCooldownReservationAndCleanup(t *testing.T) {
	store := NewCooldowns(5 * time.Minute)
	now := time.Unix(100, 0)
	if !store.TryStart(1, now) {
		t.Fatal("first TryStart() = false")
	}
	if store.TryStart(1, now) {
		t.Fatal("concurrent TryStart() = true")
	}
	store.Finish(1, now, true)
	if store.TryStart(1, now.Add(5*time.Minute-time.Nanosecond)) {
		t.Fatal("TryStart() inside cooldown = true")
	}
	if !store.TryStart(1, now.Add(5*time.Minute)) {
		t.Fatal("TryStart() at boundary = false")
	}
	store.Finish(1, now.Add(5*time.Minute), true)
	store.Cleanup(now.Add(15 * time.Minute))
	if len(store.entries) != 0 {
		t.Fatalf("entries after cleanup = %d, want 0", len(store.entries))
	}
}

func TestFailedReservationIsRemoved(t *testing.T) {
	store := NewCooldowns(time.Minute)
	now := time.Unix(100, 0)
	if !store.TryStart(1, now) {
		t.Fatal("first TryStart() = false")
	}
	store.Finish(1, now, false)
	if !store.TryStart(1, now) {
		t.Fatal("TryStart() after failure = false")
	}
}

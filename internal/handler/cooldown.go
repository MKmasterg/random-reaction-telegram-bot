package handler

import (
	"context"
	"sync"
	"time"
)

type cooldownEntry struct {
	lastSuccess time.Time
	inFlight    bool
}

// Cooldowns serializes automatic replies per group and retains only recent
// successful sends.
type Cooldowns struct {
	mu        sync.Mutex
	duration  time.Duration
	retention time.Duration
	entries   map[int64]cooldownEntry
}

func NewCooldowns(duration time.Duration) *Cooldowns {
	return &Cooldowns{
		duration:  duration,
		retention: 2 * duration,
		entries:   make(map[int64]cooldownEntry),
	}
}

// TryStart reserves a group while an automatic reply is being sent.
func (c *Cooldowns) TryStart(chatID int64, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[chatID]
	if entry.inFlight {
		return false
	}
	if exists && !entry.lastSuccess.IsZero() && now.Sub(entry.lastSuccess) < c.duration {
		return false
	}
	entry.inFlight = true
	c.entries[chatID] = entry
	return true
}

// Finish releases a reservation and records only successful deliveries.
func (c *Cooldowns) Finish(chatID int64, now time.Time, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[chatID]
	if !exists {
		return
	}
	entry.inFlight = false
	if success {
		entry.lastSuccess = now
		c.entries[chatID] = entry
		return
	}
	if entry.lastSuccess.IsZero() {
		delete(c.entries, chatID)
		return
	}
	c.entries[chatID] = entry
}

// Cleanup removes expired group state. In-flight sends are never removed.
func (c *Cooldowns) Cleanup(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for chatID, entry := range c.entries {
		if !entry.inFlight && !entry.lastSuccess.IsZero() && now.Sub(entry.lastSuccess) >= c.retention {
			delete(c.entries, chatID)
		}
	}
}

// RunCleanup periodically bounds stale map growth until ctx is canceled.
func (c *Cooldowns) RunCleanup(ctx context.Context, now func() time.Time) {
	interval := c.duration
	if interval > time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Cleanup(now())
		}
	}
}

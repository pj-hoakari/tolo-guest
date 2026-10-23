package application

import (
	"strconv"
	"testing"
	"time"
)

func TestQrOpenLimiterStopsRememberingAtCapacityUntilExpiredSessionsArePruned(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	limiter := NewQrOpenLimiter(func() time.Time { return now })

	for i := range qrOpenLimiterCapacity {
		if !limiter.Allow("event", "qr-east", strconv.Itoa(i)) {
			t.Fatalf("Allow(session %d) = false, want true for a first open", i)
		}
	}

	for range 2 {
		if !limiter.Allow("event", "qr-east", "overflow") {
			t.Fatal("Allow(overflow) = false, want a session beyond capacity counted without being remembered")
		}
	}

	now = now.Add(QrRepeatOpenWindow - time.Nanosecond)

	if limiter.Allow("event", "qr-east", "0") {
		t.Fatal("Allow(session 0) = true, want a remembered session kept while the map is full")
	}

	now = now.Add(time.Nanosecond)

	if !limiter.Allow("event", "qr-east", "overflow") {
		t.Fatal("Allow(overflow) = false, want true after the window")
	}

	if limiter.Allow("event", "qr-east", "overflow") {
		t.Fatal("Allow(overflow) = true, want the expired sessions pruned so the new session is remembered")
	}

	if got := len(limiter.countedAt); got != 1 {
		t.Errorf("remembered sessions = %d, want 1 after pruning", got)
	}
}

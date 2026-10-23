package application

import (
	"crypto/sha256"
	"sync"
	"time"
)

const (
	QrRepeatOpenWindow      = 30 * time.Minute
	qrOpenLimiterCapacity   = 100_000
	qrOpenLimiterPrunePause = time.Minute
)

type qrOpenKey struct {
	eventID      string
	qrLocationID string
	session      [sha256.Size]byte
}

// ponytail: per-process memory, so a restart forgets every session and each replica counts a session once; move to a DB table with a unique window if replicas are added.
type QrOpenLimiter struct {
	now       func() time.Time
	mu        sync.Mutex
	countedAt map[qrOpenKey]time.Time
	prunedAt  time.Time
}

func NewQrOpenLimiter(now func() time.Time) *QrOpenLimiter {
	return &QrOpenLimiter{now: now, mu: sync.Mutex{}, countedAt: map[qrOpenKey]time.Time{}, prunedAt: time.Time{}}
}

func (l *QrOpenLimiter) Allow(eventID, qrLocationID, sessionID string) bool {
	if sessionID == "" {
		return false
	}

	key := qrOpenKey{eventID: eventID, qrLocationID: qrLocationID, session: sha256.Sum256([]byte(sessionID))}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if countedAt, ok := l.countedAt[key]; ok && now.Sub(countedAt) < QrRepeatOpenWindow {
		return false
	}

	if len(l.countedAt) >= qrOpenLimiterCapacity && now.Sub(l.prunedAt) >= qrOpenLimiterPrunePause {
		l.prune(now)
	}

	if len(l.countedAt) < qrOpenLimiterCapacity {
		l.countedAt[key] = now
	}

	return true
}

func (l *QrOpenLimiter) prune(now time.Time) {
	for key, countedAt := range l.countedAt {
		if now.Sub(countedAt) >= QrRepeatOpenWindow {
			delete(l.countedAt, key)
		}
	}

	l.prunedAt = now
}

package db

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

func TestGuestReadModelStatus(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()
	takenAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

	status := func(sequence uint64, level domain.CrowdLevel) domain.GuestStatus {
		return domain.GuestStatus{
			EventID:  "read-model-status-event",
			Sequence: sequence,
			TakenAt:  takenAt.Add(time.Duration(sequence) * time.Minute),
			Crowd: []domain.CrowdStatus{
				{Target: domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: "p1"}, Level: level, Danger: false},
				{Target: domain.CrowdTarget{Kind: domain.CrowdTargetRoute, ID: "r1"}, Level: domain.CrowdLevelHigh, Danger: true},
			},
			Queues: []domain.QueueStatus{{
				QueueID:                 "q1",
				GatePointIDs:            []string{"g1", "g2"},
				WaitTimeMinutes:         12.5,
				TurnaroundCount:         3,
				RecommendedGatePointIDs: []string{"g2"},
			}},
		}
	}

	assertStatus := func(t *testing.T, want domain.GuestStatus) {
		t.Helper()

		got, err := repo.FindStatus(ctx, want.EventID)
		if err != nil {
			t.Fatalf("FindStatus() error = %v", err)
		}

		if !got.TakenAt.Equal(want.TakenAt) {
			t.Errorf("FindStatus() taken_at = %v, want %v", got.TakenAt, want.TakenAt)
		}

		got.TakenAt = want.TakenAt
		if !reflect.DeepEqual(got, want) {
			t.Errorf("FindStatus() = %+v, want %+v", got, want)
		}
	}

	t.Run("reports an event without a status as not found", func(t *testing.T) {
		if _, err := repo.FindStatus(ctx, "read-model-status-unknown"); !errors.Is(err, repository.ErrGuestStatusNotFound) {
			t.Errorf("FindStatus() error = %v, want %v", err, repository.ErrGuestStatusNotFound)
		}
	})

	t.Run("replaces the status with a newer sequence", func(t *testing.T) {
		for _, s := range []domain.GuestStatus{status(1, domain.CrowdLevelLow), status(2, domain.CrowdLevelMid)} {
			if err := repo.ApplyStatus(ctx, s); err != nil {
				t.Fatalf("ApplyStatus(%d) error = %v", s.Sequence, err)
			}
		}

		assertStatus(t, status(2, domain.CrowdLevelMid))
	})

	t.Run("discards a stale or duplicate sequence", func(t *testing.T) {
		for _, s := range []domain.GuestStatus{status(1, domain.CrowdLevelHigh), status(2, domain.CrowdLevelHigh)} {
			if err := repo.ApplyStatus(ctx, s); err != nil {
				t.Fatalf("ApplyStatus(%d) error = %v", s.Sequence, err)
			}
		}

		assertStatus(t, status(2, domain.CrowdLevelMid))
	})
}

func TestGuestReadModelMessages(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()
	sentAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

	message := func(id, body string, minute int) domain.GuestMessage {
		return domain.GuestMessage{MessageID: id, Body: body, SentAt: sentAt.Add(time.Duration(minute) * time.Minute)}
	}

	assertMessages := func(t *testing.T, eventID string, want ...domain.GuestMessage) {
		t.Helper()

		got, err := repo.ListMessages(ctx, eventID)
		if err != nil {
			t.Fatalf("ListMessages() error = %v", err)
		}

		equal := slices.EqualFunc(got, want, func(a, b domain.GuestMessage) bool {
			return a.MessageID == b.MessageID && a.Body == b.Body && a.SentAt.Equal(b.SentAt)
		})
		if !equal {
			t.Errorf("ListMessages(%s) = %+v, want %+v", eventID, got, want)
		}
	}

	t.Run("keeps one copy of a redelivered update", func(t *testing.T) {
		const eventID = "read-model-messages-duplicate"

		update := domain.GuestMessagesUpdate{
			EventID:           eventID,
			Upserts:           []domain.GuestMessage{message("m1", "Gate A is open", 0), message("m2", "Gate B is closed", 1)},
			RevokedMessageIDs: nil,
		}

		for range 2 {
			if err := repo.ApplyMessages(ctx, update); err != nil {
				t.Fatalf("ApplyMessages() error = %v", err)
			}
		}

		assertMessages(t, eventID, message("m1", "Gate A is open", 0), message("m2", "Gate B is closed", 1))
	})

	t.Run("removes revoked messages of the event only", func(t *testing.T) {
		const eventID = "read-model-messages-revoke"

		for _, update := range []domain.GuestMessagesUpdate{
			{EventID: eventID, Upserts: []domain.GuestMessage{message("m1", "one", 0), message("m2", "two", 1)}, RevokedMessageIDs: nil},
			{EventID: "read-model-messages-revoke-other", Upserts: []domain.GuestMessage{message("m1", "other", 0)}, RevokedMessageIDs: nil},
			{EventID: eventID, Upserts: []domain.GuestMessage{message("m3", "three", 2)}, RevokedMessageIDs: []string{"m1", "unknown"}},
		} {
			if err := repo.ApplyMessages(ctx, update); err != nil {
				t.Fatalf("ApplyMessages() error = %v", err)
			}
		}

		assertMessages(t, eventID, message("m2", "two", 1), message("m3", "three", 2))
		assertMessages(t, "read-model-messages-revoke-other", message("m1", "other", 0))
	})

	t.Run("treats stored messages missing from the full set as revoked", func(t *testing.T) {
		const eventID = "read-model-messages-replace"

		if err := repo.ApplyMessages(ctx, domain.GuestMessagesUpdate{
			EventID:           eventID,
			Upserts:           []domain.GuestMessage{message("m1", "one", 0), message("m2", "two", 1)},
			RevokedMessageIDs: nil,
		}); err != nil {
			t.Fatalf("ApplyMessages() error = %v", err)
		}

		if err := repo.ReplaceMessages(ctx, eventID, []domain.GuestMessage{message("m2", "two edited", 1), message("m4", "four", 3)}); err != nil {
			t.Fatalf("ReplaceMessages() error = %v", err)
		}

		assertMessages(t, eventID, message("m2", "two edited", 1), message("m4", "four", 3))

		if err := repo.ReplaceMessages(ctx, eventID, nil); err != nil {
			t.Fatalf("ReplaceMessages(nil) error = %v", err)
		}

		assertMessages(t, eventID)
	})
}

func TestGuestReadModelListEventIDs(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()

	if err := repo.ApplyStatus(ctx, domain.GuestStatus{
		EventID: "read-model-list-status", Sequence: 1, TakenAt: time.Now(), Crowd: nil, Queues: nil,
	}); err != nil {
		t.Fatalf("ApplyStatus() error = %v", err)
	}

	if err := repo.ApplyMessages(ctx, domain.GuestMessagesUpdate{
		EventID:           "read-model-list-messages",
		Upserts:           []domain.GuestMessage{{MessageID: "m1", Body: "hello", SentAt: time.Now()}},
		RevokedMessageIDs: nil,
	}); err != nil {
		t.Fatalf("ApplyMessages() error = %v", err)
	}

	got, err := repo.ListEventIDs(ctx)
	if err != nil {
		t.Fatalf("ListEventIDs() error = %v", err)
	}

	for _, want := range []string{"read-model-list-status", "read-model-list-messages"} {
		if !slices.Contains(got, want) {
			t.Errorf("ListEventIDs() = %v, want it to contain %s", got, want)
		}
	}
}

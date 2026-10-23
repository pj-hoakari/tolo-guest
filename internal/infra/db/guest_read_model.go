package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

var crowdLevelValues = map[domain.CrowdLevel]string{
	domain.CrowdLevelUnspecified: "unspecified",
	domain.CrowdLevelLow:         "low",
	domain.CrowdLevelMid:         "mid",
	domain.CrowdLevelHigh:        "high",
}

type PostgresGuestReadModelRepository struct {
	db *sqlx.DB
}

func NewPostgresGuestReadModelRepository(db *sqlx.DB) *PostgresGuestReadModelRepository {
	return &PostgresGuestReadModelRepository{db: db}
}

type crowdStatusDocument struct {
	PointID string `json:"point_id,omitempty"`
	RouteID string `json:"route_id,omitempty"`
	Level   string `json:"level"`
	Danger  bool   `json:"danger"`
}

type queueStatusDocument struct {
	QueueID                 string   `json:"queue_id"`
	GatePointIDs            []string `json:"gate_point_ids"`
	WaitTimeMinutes         float64  `json:"wait_time_minutes"`
	TurnaroundCount         int32    `json:"turnaround_count"`
	RecommendedGatePointIDs []string `json:"recommended_gate_point_ids"`
}

type guestStatusRow struct {
	Sequence uint64    `db:"sequence"`
	TakenAt  time.Time `db:"taken_at"`
	Crowd    []byte    `db:"crowd"`
	Queues   []byte    `db:"queues"`
}

func (r *PostgresGuestReadModelRepository) ApplyStatus(ctx context.Context, status domain.GuestStatus) error {
	crowd := make([]crowdStatusDocument, 0, len(status.Crowd))
	for _, c := range status.Crowd {
		document := crowdStatusDocument{PointID: "", RouteID: "", Level: crowdLevelValues[c.Level], Danger: c.Danger}

		switch c.Target.Kind {
		case domain.CrowdTargetPoint:
			document.PointID = c.Target.ID
		case domain.CrowdTargetRoute:
			document.RouteID = c.Target.ID
		case domain.CrowdTargetUnspecified:
		}

		crowd = append(crowd, document)
	}

	queues := make([]queueStatusDocument, 0, len(status.Queues))
	for _, q := range status.Queues {
		queues = append(queues, queueStatusDocument(q))
	}

	encodedCrowd, err := json.Marshal(crowd)
	if err != nil {
		return fmt.Errorf("encode guest crowd status: %w", err)
	}

	encodedQueues, err := json.Marshal(queues)
	if err != nil {
		return fmt.Errorf("encode guest queue status: %w", err)
	}

	if _, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_statuses AS s (event_id, sequence, taken_at, crowd, queues)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO UPDATE
		SET sequence = EXCLUDED.sequence,
			taken_at = EXCLUDED.taken_at,
			crowd = EXCLUDED.crowd,
			queues = EXCLUDED.queues,
			updated_at = now()
		WHERE EXCLUDED.sequence > s.sequence`,
		status.EventID, status.Sequence, status.TakenAt, encodedCrowd, encodedQueues); err != nil {
		return fmt.Errorf("upsert guest status: %w", err)
	}

	return nil
}

func (r *PostgresGuestReadModelRepository) FindStatus(ctx context.Context, eventID string) (domain.GuestStatus, error) {
	var row guestStatusRow

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &row, `
		SELECT sequence, taken_at, crowd, queues
		FROM guest_statuses
		WHERE event_id = $1`, eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GuestStatus{}, repository.ErrGuestStatusNotFound
	}

	if err != nil {
		return domain.GuestStatus{}, fmt.Errorf("select guest status: %w", err)
	}

	var (
		crowd  []crowdStatusDocument
		queues []queueStatusDocument
	)

	if err := json.Unmarshal(row.Crowd, &crowd); err != nil {
		return domain.GuestStatus{}, fmt.Errorf("decode guest crowd status: %w", err)
	}

	if err := json.Unmarshal(row.Queues, &queues); err != nil {
		return domain.GuestStatus{}, fmt.Errorf("decode guest queue status: %w", err)
	}

	status := domain.GuestStatus{
		EventID:  eventID,
		Sequence: row.Sequence,
		TakenAt:  row.TakenAt,
		Crowd:    make([]domain.CrowdStatus, 0, len(crowd)),
		Queues:   make([]domain.QueueStatus, 0, len(queues)),
	}

	for _, c := range crowd {
		level, ok := storedValue(crowdLevelValues, c.Level)
		if !ok {
			return domain.GuestStatus{}, fmt.Errorf("unknown stored crowd level %q", c.Level)
		}

		target := domain.CrowdTarget{Kind: domain.CrowdTargetUnspecified, ID: ""}

		switch {
		case c.PointID != "":
			target = domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: c.PointID}
		case c.RouteID != "":
			target = domain.CrowdTarget{Kind: domain.CrowdTargetRoute, ID: c.RouteID}
		}

		status.Crowd = append(status.Crowd, domain.CrowdStatus{Target: target, Level: level, Danger: c.Danger})
	}

	for _, q := range queues {
		status.Queues = append(status.Queues, domain.QueueStatus(q))
	}

	return status, nil
}

func (r *PostgresGuestReadModelRepository) ApplyMessages(ctx context.Context, update domain.GuestMessagesUpdate) error {
	return RunInTransaction(ctx, r.db, func(ctx context.Context) error {
		if err := r.upsertMessages(ctx, update.EventID, update.Upserts); err != nil {
			return err
		}

		if _, err := Executor(ctx, r.db).ExecContext(ctx, `
			DELETE FROM guest_messages
			WHERE event_id = $1 AND message_id = ANY($2)`,
			update.EventID, append([]string{}, update.RevokedMessageIDs...)); err != nil {
			return fmt.Errorf("delete revoked guest messages: %w", err)
		}

		return nil
	})
}

func (r *PostgresGuestReadModelRepository) ReplaceMessages(
	ctx context.Context,
	eventID string,
	messages []domain.GuestMessage,
) error {
	return RunInTransaction(ctx, r.db, func(ctx context.Context) error {
		if err := r.upsertMessages(ctx, eventID, messages); err != nil {
			return err
		}

		ids := make([]string, 0, len(messages))
		for _, message := range messages {
			ids = append(ids, message.MessageID)
		}

		if _, err := Executor(ctx, r.db).ExecContext(ctx, `
			DELETE FROM guest_messages
			WHERE event_id = $1 AND message_id <> ALL($2)`,
			eventID, ids); err != nil {
			return fmt.Errorf("delete inactive guest messages: %w", err)
		}

		return nil
	})
}

func (r *PostgresGuestReadModelRepository) ListMessages(ctx context.Context, eventID string) ([]domain.GuestMessage, error) {
	var rows []struct {
		MessageID string    `db:"message_id"`
		Body      string    `db:"body"`
		SentAt    time.Time `db:"sent_at"`
	}

	if err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT message_id, body, sent_at
		FROM guest_messages
		WHERE event_id = $1
		ORDER BY sent_at, message_id`, eventID); err != nil {
		return nil, fmt.Errorf("select guest messages: %w", err)
	}

	messages := make([]domain.GuestMessage, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, domain.GuestMessage{MessageID: row.MessageID, Body: row.Body, SentAt: row.SentAt})
	}

	return messages, nil
}

func (r *PostgresGuestReadModelRepository) ListEventIDs(ctx context.Context) ([]string, error) {
	var eventIDs []string

	if err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &eventIDs, `
		SELECT event_id FROM guest_statuses
		UNION
		SELECT event_id FROM guest_messages
		ORDER BY event_id`); err != nil {
		return nil, fmt.Errorf("select read model events: %w", err)
	}

	return eventIDs, nil
}

func (r *PostgresGuestReadModelRepository) upsertMessages(
	ctx context.Context,
	eventID string,
	messages []domain.GuestMessage,
) error {
	ids := make([]string, 0, len(messages))
	bodies := make([]string, 0, len(messages))
	sentAts := make([]time.Time, 0, len(messages))

	for _, message := range messages {
		ids = append(ids, message.MessageID)
		bodies = append(bodies, message.Body)
		sentAts = append(sentAts, message.SentAt)
	}

	if _, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_messages (event_id, message_id, body, sent_at)
		SELECT $1, message_id, body, sent_at
		FROM unnest($2::text[], $3::text[], $4::timestamptz[]) AS m (message_id, body, sent_at)
		ON CONFLICT (event_id, message_id) DO UPDATE
		SET body = EXCLUDED.body, sent_at = EXCLUDED.sent_at`,
		eventID, ids, bodies, sentAts); err != nil {
		return fmt.Errorf("upsert guest messages: %w", err)
	}

	return nil
}

func storedValue[K comparable](values map[K]string, value string) (K, bool) {
	for key, stored := range values {
		if stored == value {
			return key, true
		}
	}

	var zero K

	return zero, false
}

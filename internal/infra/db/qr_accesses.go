package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

type PostgresQrAccessRepository struct {
	db *sqlx.DB
}

func NewPostgresQrAccessRepository(db *sqlx.DB) *PostgresQrAccessRepository {
	return &PostgresQrAccessRepository{db: db}
}

func (r *PostgresQrAccessRepository) RecordQrAccess(ctx context.Context, access domain.QrAccess) error {
	if _, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_qr_accesses (event_id, qr_location_id, tenant_public_id)
		VALUES ($1, $2, $3)`,
		access.EventID, access.QrLocationID, access.TenantPublicID); err != nil {
		return fmt.Errorf("insert QR access: %w", err)
	}

	return nil
}

func (r *PostgresQrAccessRepository) ListUnreportedQrAccesses(ctx context.Context, openedBefore time.Time) ([]domain.RecordedQrAccess, error) {
	var rows []struct {
		ID             int64     `db:"id"`
		EventID        string    `db:"event_id"`
		QrLocationID   string    `db:"qr_location_id"`
		TenantPublicID string    `db:"tenant_public_id"`
		OpenedAt       time.Time `db:"opened_at"`
	}

	if err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT id, event_id, qr_location_id, tenant_public_id, opened_at
		FROM guest_qr_accesses
		WHERE opened_at < $1
		ORDER BY id`, openedBefore); err != nil {
		return nil, fmt.Errorf("select unreported QR accesses: %w", err)
	}

	accesses := make([]domain.RecordedQrAccess, 0, len(rows))
	for _, row := range rows {
		accesses = append(accesses, domain.RecordedQrAccess{
			ID:       row.ID,
			Access:   domain.QrAccess{EventID: row.EventID, QrLocationID: row.QrLocationID, TenantPublicID: row.TenantPublicID},
			OpenedAt: row.OpenedAt,
		})
	}

	return accesses, nil
}

func (r *PostgresQrAccessRepository) DeleteQrAccesses(ctx context.Context, ids []int64) error {
	if _, err := Executor(ctx, r.db).ExecContext(ctx, `DELETE FROM guest_qr_accesses WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("delete QR accesses: %w", err)
	}

	return nil
}

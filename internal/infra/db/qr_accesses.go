package db

import (
	"context"
	"fmt"

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

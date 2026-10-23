package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
	"github.com/pj-hoakari/tolo-guest/internal/tenantctx"
)

var componentValues = map[domain.Component]string{
	domain.ComponentCrowd:         "crowd",
	domain.ComponentQueues:        "queues",
	domain.ComponentStaffMessages: "staff_messages",
}

type PostgresGuestSettingsRepository struct {
	db *sqlx.DB
}

func NewPostgresGuestSettingsRepository(db *sqlx.DB) *PostgresGuestSettingsRepository {
	return &PostgresGuestSettingsRepository{db: db}
}

type guestSettingsRow struct {
	TenantPublicID   string `db:"tenant_public_id"`
	PageCompositions []byte `db:"page_compositions"`
}

type pageCompositionDocument struct {
	Kind       string   `json:"kind"`
	Components []string `json:"components"`
}

func (r *PostgresGuestSettingsRepository) Find(ctx context.Context, eventID string) (domain.GuestSettings, error) {
	var row guestSettingsRow

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &row, `
		SELECT tenant_public_id, page_compositions
		FROM guest_settings
		WHERE event_id = $1`, eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GuestSettings{}, repository.ErrGuestSettingsNotFound
	}

	if err != nil {
		return domain.GuestSettings{}, fmt.Errorf("select guest settings: %w", err)
	}

	if err := tenantctx.VerifyOwnership(ctx, row.TenantPublicID); err != nil {
		return domain.GuestSettings{}, err
	}

	var documents []pageCompositionDocument
	if err := json.Unmarshal(row.PageCompositions, &documents); err != nil {
		return domain.GuestSettings{}, fmt.Errorf("decode page compositions: %w", err)
	}

	compositions := make([]domain.PageComposition, 0, len(documents))

	for _, document := range documents {
		components := make([]domain.Component, 0, len(document.Components))

		for _, value := range document.Components {
			component, err := storedComponent(value)
			if err != nil {
				return domain.GuestSettings{}, err
			}

			components = append(components, component)
		}

		compositions = append(compositions, domain.PageComposition{Kind: document.Kind, Components: components})
	}

	return domain.GuestSettings{EventID: eventID, TenantPublicID: row.TenantPublicID, PageCompositions: compositions}, nil
}

func (r *PostgresGuestSettingsRepository) Save(ctx context.Context, settings domain.GuestSettings) error {
	documents := make([]pageCompositionDocument, 0, len(settings.PageCompositions))

	for _, composition := range settings.PageCompositions {
		components := make([]string, 0, len(composition.Components))
		for _, component := range composition.Components {
			components = append(components, componentValues[component])
		}

		documents = append(documents, pageCompositionDocument{Kind: composition.Kind, Components: components})
	}

	compositions, err := json.Marshal(documents)
	if err != nil {
		return fmt.Errorf("encode page compositions: %w", err)
	}

	result, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_settings (event_id, tenant_public_id, page_compositions)
		VALUES ($1, $2, $3)
		ON CONFLICT (event_id) DO UPDATE
		SET page_compositions = EXCLUDED.page_compositions, updated_at = now()
		WHERE guest_settings.tenant_public_id = EXCLUDED.tenant_public_id`,
		settings.EventID, settings.TenantPublicID, compositions)
	if err != nil {
		return fmt.Errorf("upsert guest settings: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count upserted guest settings: %w", err)
	}

	if affected == 0 {
		return tenantctx.ErrMismatch
	}

	return nil
}

func (r *PostgresGuestSettingsRepository) ListEventTenants(ctx context.Context) ([]domain.EventTenant, error) {
	var rows []struct {
		EventID        string `db:"event_id"`
		TenantPublicID string `db:"tenant_public_id"`
	}

	if err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT event_id, tenant_public_id
		FROM guest_settings
		ORDER BY event_id`); err != nil {
		return nil, fmt.Errorf("select event tenants: %w", err)
	}

	tenants := make([]domain.EventTenant, 0, len(rows))
	for _, row := range rows {
		tenants = append(tenants, domain.EventTenant{EventID: row.EventID, TenantPublicID: row.TenantPublicID})
	}

	return tenants, nil
}

func storedComponent(value string) (domain.Component, error) {
	for component, stored := range componentValues {
		if stored == value {
			return component, nil
		}
	}

	return domain.ComponentUnspecified, fmt.Errorf("unknown stored component %q", value)
}

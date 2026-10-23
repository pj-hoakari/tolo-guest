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
)

type qrLocationDocument struct {
	QrLocationID string `json:"qr_location_id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
}

type qrLocationsRow struct {
	RevisionID  string `db:"revision_id"`
	QrLocations []byte `db:"qr_locations"`
}

func (r *PostgresGuestReadModelRepository) SaveQrLocations(ctx context.Context, locations domain.QrLocations) error {
	documents := make([]qrLocationDocument, 0, len(locations.Locations))
	for _, location := range locations.Locations {
		documents = append(documents, qrLocationDocument(location))
	}

	encoded, err := json.Marshal(documents)
	if err != nil {
		return fmt.Errorf("encode QR locations: %w", err)
	}

	if _, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_qr_locations (event_id, revision_id, qr_locations)
		VALUES ($1, $2, $3)
		ON CONFLICT (event_id) DO UPDATE
		SET revision_id = EXCLUDED.revision_id,
			qr_locations = EXCLUDED.qr_locations,
			updated_at = now()`,
		locations.EventID, locations.RevisionID, encoded); err != nil {
		return fmt.Errorf("upsert QR locations: %w", err)
	}

	return nil
}

func (r *PostgresGuestReadModelRepository) FindQrLocations(ctx context.Context, eventID string) (domain.QrLocations, error) {
	var row qrLocationsRow

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &row, `
		SELECT revision_id, qr_locations
		FROM guest_qr_locations
		WHERE event_id = $1`, eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.QrLocations{}, repository.ErrQrLocationsNotFound
	}

	if err != nil {
		return domain.QrLocations{}, fmt.Errorf("select QR locations: %w", err)
	}

	var documents []qrLocationDocument
	if err := json.Unmarshal(row.QrLocations, &documents); err != nil {
		return domain.QrLocations{}, fmt.Errorf("decode QR locations: %w", err)
	}

	locations := domain.QrLocations{
		EventID:    eventID,
		RevisionID: row.RevisionID,
		Locations:  make([]domain.QrLocation, 0, len(documents)),
	}

	for _, document := range documents {
		locations.Locations = append(locations.Locations, domain.QrLocation(document))
	}

	return locations, nil
}

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

type routeEndpointsDocument struct {
	FromPointID string `json:"from_point_id"`
	ToPointID   string `json:"to_point_id"`
}

type displayNamesDocument struct {
	Points         map[string]domain.LocalizedText   `json:"points"`
	Routes         map[string]string                 `json:"routes"`
	RouteEndpoints map[string]routeEndpointsDocument `json:"route_endpoints"`
}

type displayNamesRow struct {
	RevisionID   string `db:"revision_id"`
	DisplayNames []byte `db:"display_names"`
}

func (r *PostgresGuestReadModelRepository) SaveDisplayNames(ctx context.Context, names domain.DisplayNames) error {
	document := displayNamesDocument{
		Points:         names.Points,
		Routes:         names.Routes,
		RouteEndpoints: make(map[string]routeEndpointsDocument, len(names.RouteEndpoints)),
	}

	for routeID, endpoints := range names.RouteEndpoints {
		document.RouteEndpoints[routeID] = routeEndpointsDocument(endpoints)
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode display names: %w", err)
	}

	if _, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_display_names (event_id, revision_id, display_names)
		VALUES ($1, $2, $3)
		ON CONFLICT (event_id) DO UPDATE
		SET revision_id = EXCLUDED.revision_id,
			display_names = EXCLUDED.display_names,
			updated_at = now()`,
		names.EventID, names.RevisionID, encoded); err != nil {
		return fmt.Errorf("upsert display names: %w", err)
	}

	return nil
}

func (r *PostgresGuestReadModelRepository) FindDisplayNames(ctx context.Context, eventID string) (domain.DisplayNames, error) {
	var row displayNamesRow

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &row, `
		SELECT revision_id, display_names
		FROM guest_display_names
		WHERE event_id = $1`, eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DisplayNames{}, repository.ErrDisplayNamesNotFound
	}

	if err != nil {
		return domain.DisplayNames{}, fmt.Errorf("select display names: %w", err)
	}

	var document displayNamesDocument
	if err := json.Unmarshal(row.DisplayNames, &document); err != nil {
		return domain.DisplayNames{}, fmt.Errorf("decode display names: %w", err)
	}

	names := domain.DisplayNames{
		EventID:        eventID,
		RevisionID:     row.RevisionID,
		Points:         document.Points,
		Routes:         document.Routes,
		RouteEndpoints: make(map[string]domain.RouteEndpoints, len(document.RouteEndpoints)),
	}

	for routeID, endpoints := range document.RouteEndpoints {
		names.RouteEndpoints[routeID] = domain.RouteEndpoints(endpoints)
	}

	return names, nil
}

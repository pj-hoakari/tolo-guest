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

type guideMapRow struct {
	RevisionID  string `db:"revision_id"`
	Layout      []byte `db:"layout"`
	PointLabels []byte `db:"point_labels"`
	GroupLabels []byte `db:"group_labels"`
}

func (r *PostgresGuestReadModelRepository) SaveGuideMap(ctx context.Context, guideMap domain.GuideMap) error {
	points, err := json.Marshal(guideMap.Points)
	if err != nil {
		return fmt.Errorf("encode guide map point labels: %w", err)
	}

	groups, err := json.Marshal(guideMap.Groups)
	if err != nil {
		return fmt.Errorf("encode guide map group labels: %w", err)
	}

	if _, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guest_guide_maps (event_id, revision_id, layout, point_labels, group_labels)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO UPDATE
		SET revision_id = EXCLUDED.revision_id,
			layout = EXCLUDED.layout,
			point_labels = EXCLUDED.point_labels,
			group_labels = EXCLUDED.group_labels,
			updated_at = now()`,
		guideMap.EventID, guideMap.RevisionID, guideMap.EncodedLayout, points, groups); err != nil {
		return fmt.Errorf("upsert guide map: %w", err)
	}

	return nil
}

func (r *PostgresGuestReadModelRepository) FindGuideMap(ctx context.Context, eventID string) (domain.GuideMap, error) {
	var row guideMapRow

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &row, `
		SELECT revision_id, layout, point_labels, group_labels
		FROM guest_guide_maps
		WHERE event_id = $1`, eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GuideMap{}, repository.ErrGuideMapNotFound
	}

	if err != nil {
		return domain.GuideMap{}, fmt.Errorf("select guide map: %w", err)
	}

	guideMap := domain.GuideMap{EventID: eventID, RevisionID: row.RevisionID, EncodedLayout: row.Layout, Points: nil, Groups: nil}

	if err := json.Unmarshal(row.PointLabels, &guideMap.Points); err != nil {
		return domain.GuideMap{}, fmt.Errorf("decode guide map point labels: %w", err)
	}

	if err := json.Unmarshal(row.GroupLabels, &guideMap.Groups); err != nil {
		return domain.GuideMap{}, fmt.Errorf("decode guide map group labels: %w", err)
	}

	return guideMap, nil
}

func (r *PostgresGuestReadModelRepository) DeleteGuideMap(ctx context.Context, eventID string) error {
	if _, err := Executor(ctx, r.db).ExecContext(ctx, `DELETE FROM guest_guide_maps WHERE event_id = $1`, eventID); err != nil {
		return fmt.Errorf("delete guide map: %w", err)
	}

	return nil
}

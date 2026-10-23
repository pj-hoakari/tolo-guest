package db

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

func TestDisplayNames(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()

	names := func(revisionID, eastLabel string) domain.DisplayNames {
		return domain.DisplayNames{
			EventID:    "display-names-event",
			RevisionID: revisionID,
			Points: map[string]domain.LocalizedText{
				"p-east": {"ja": eastLabel, "en": "East Gate"},
				"p-west": {"ja": "西門"},
			},
			Routes:         map[string]string{"r1": "メイン通り"},
			RouteEndpoints: map[string]domain.RouteEndpoints{"r1": {FromPointID: "p-east", ToPointID: "p-west"}},
		}
	}

	assertNames := func(t *testing.T, want domain.DisplayNames) {
		t.Helper()

		got, err := repo.FindDisplayNames(ctx, want.EventID)
		if err != nil {
			t.Fatalf("FindDisplayNames() error = %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("FindDisplayNames() = %+v, want %+v", got, want)
		}
	}

	t.Run("reports an event without display names as not found", func(t *testing.T) {
		if _, err := repo.FindDisplayNames(ctx, "display-names-unknown"); !errors.Is(err, repository.ErrDisplayNamesNotFound) {
			t.Errorf("FindDisplayNames() error = %v, want %v", err, repository.ErrDisplayNamesNotFound)
		}
	})

	t.Run("keeps the pulled names with their revision", func(t *testing.T) {
		if err := repo.SaveDisplayNames(ctx, names("rev-1", "東門")); err != nil {
			t.Fatalf("SaveDisplayNames() error = %v", err)
		}

		assertNames(t, names("rev-1", "東門"))
	})

	t.Run("replaces the names and the revision with the latest pull", func(t *testing.T) {
		if err := repo.SaveDisplayNames(ctx, names("rev-2", "東ゲート")); err != nil {
			t.Fatalf("SaveDisplayNames() error = %v", err)
		}

		assertNames(t, names("rev-2", "東ゲート"))
	})
}

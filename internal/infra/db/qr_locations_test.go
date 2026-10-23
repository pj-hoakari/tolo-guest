package db

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

func TestQrLocations(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()

	assertLocations := func(t *testing.T, want domain.QrLocations) {
		t.Helper()

		got, err := repo.FindQrLocations(ctx, want.EventID)
		if err != nil {
			t.Fatalf("FindQrLocations() error = %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("FindQrLocations() = %+v, want %+v", got, want)
		}
	}

	first := domain.QrLocations{
		EventID:    "qr-locations-event",
		RevisionID: "rev-1",
		Locations: []domain.QrLocation{
			{QrLocationID: "qr-east", Name: "東門", Kind: "entrance"},
			{QrLocationID: "qr-hall", Name: "ホール前", Kind: "queue"},
		},
	}

	t.Run("reports an event without QR locations as not found", func(t *testing.T) {
		if _, err := repo.FindQrLocations(ctx, "qr-locations-unknown"); !errors.Is(err, repository.ErrQrLocationsNotFound) {
			t.Errorf("FindQrLocations() error = %v, want %v", err, repository.ErrQrLocationsNotFound)
		}
	})

	t.Run("keeps the pulled locations in order with their revision", func(t *testing.T) {
		if err := repo.SaveQrLocations(ctx, first); err != nil {
			t.Fatalf("SaveQrLocations() error = %v", err)
		}

		assertLocations(t, first)
	})

	t.Run("replaces the locations and the revision with the latest pull", func(t *testing.T) {
		second := domain.QrLocations{
			EventID:    first.EventID,
			RevisionID: "rev-2",
			Locations:  []domain.QrLocation{{QrLocationID: "qr-west", Name: "西門", Kind: "entrance"}},
		}

		if err := repo.SaveQrLocations(ctx, second); err != nil {
			t.Fatalf("SaveQrLocations() error = %v", err)
		}

		assertLocations(t, second)
	})

	t.Run("keeps a revision without locations as an empty list", func(t *testing.T) {
		empty := domain.QrLocations{EventID: first.EventID, RevisionID: "rev-3", Locations: []domain.QrLocation{}}

		if err := repo.SaveQrLocations(ctx, empty); err != nil {
			t.Fatalf("SaveQrLocations() error = %v", err)
		}

		assertLocations(t, empty)
	})
}

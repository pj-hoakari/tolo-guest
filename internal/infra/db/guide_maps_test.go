package db

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

func TestGuestReadModelGuideMap(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()
	eventID := "read-model-guide-map-event"

	if _, err := repo.FindGuideMap(ctx, eventID); !errors.Is(err, repository.ErrGuideMapNotFound) {
		t.Fatalf("FindGuideMap() before saving error = %v, want %v", err, repository.ErrGuideMapNotFound)
	}

	first := domain.GuideMap{
		EventID:       eventID,
		RevisionID:    "rev-1",
		EncodedLayout: []byte{0x0a, 0x02, 0x08, 0x01},
		Points:        map[string]domain.LocalizedText{"p-hall": {"ja": "ホール", "en": "Hall"}},
		Groups:        map[string]domain.LocalizedText{"g-1f": {"ja": "1階"}},
	}
	second := domain.GuideMap{
		EventID:       eventID,
		RevisionID:    "rev-2",
		EncodedLayout: []byte{0x12, 0x00},
		Points:        map[string]domain.LocalizedText{},
		Groups:        map[string]domain.LocalizedText{"g-2f": {"ja": "2階"}},
	}

	for _, want := range []domain.GuideMap{first, second} {
		if err := repo.SaveGuideMap(ctx, want); err != nil {
			t.Fatalf("SaveGuideMap(%s) error = %v", want.RevisionID, err)
		}

		got, err := repo.FindGuideMap(ctx, eventID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("FindGuideMap() = %+v, %v, want %+v", got, err, want)
		}
	}

	if err := repo.DeleteGuideMap(ctx, eventID); err != nil {
		t.Fatalf("DeleteGuideMap() error = %v", err)
	}

	if _, err := repo.FindGuideMap(ctx, eventID); !errors.Is(err, repository.ErrGuideMapNotFound) {
		t.Errorf("FindGuideMap() after deleting error = %v, want %v", err, repository.ErrGuideMapNotFound)
	}
}

func TestGuestReadModelQrLocationsKeepTheirPoint(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestReadModelRepository(testDB)
	ctx := context.Background()
	want := domain.QrLocations{
		EventID:    "read-model-qr-point-event",
		RevisionID: "rev-1",
		Locations: []domain.QrLocation{
			{QrLocationID: "qr-hall", Name: "ホール前", Kind: "poster", PointID: "p-hall"},
			{QrLocationID: "qr-corridor", Name: "廊下", Kind: "poster", PointID: ""},
		},
	}

	if err := repo.SaveQrLocations(ctx, want); err != nil {
		t.Fatalf("SaveQrLocations() error = %v", err)
	}

	if got, err := repo.FindQrLocations(ctx, want.EventID); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FindQrLocations() = %+v, %v, want %+v", got, err, want)
	}
}

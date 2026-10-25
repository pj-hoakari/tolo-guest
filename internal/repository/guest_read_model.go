package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

var (
	ErrGuestStatusNotFound  = errors.New("guest status not found")
	ErrDisplayNamesNotFound = errors.New("display names not found")
	ErrQrLocationsNotFound  = errors.New("QR locations not found")
	ErrGuideMapNotFound     = errors.New("guide map not found")
)

type GuestReadModelRepository interface {
	ApplyStatus(ctx context.Context, status domain.GuestStatus) error
	ApplyMessages(ctx context.Context, update domain.GuestMessagesUpdate) error
	ReplaceMessages(ctx context.Context, eventID string, messages []domain.GuestMessage) error
	ListEventIDs(ctx context.Context) ([]string, error)
	FindStatus(ctx context.Context, eventID string) (domain.GuestStatus, error)
	ListMessages(ctx context.Context, eventID string) ([]domain.GuestMessage, error)
	SaveDisplayNames(ctx context.Context, names domain.DisplayNames) error
	FindDisplayNames(ctx context.Context, eventID string) (domain.DisplayNames, error)
	SaveQrLocations(ctx context.Context, locations domain.QrLocations) error
	FindQrLocations(ctx context.Context, eventID string) (domain.QrLocations, error)
	SaveGuideMap(ctx context.Context, guideMap domain.GuideMap) error
	FindGuideMap(ctx context.Context, eventID string) (domain.GuideMap, error)
	DeleteGuideMap(ctx context.Context, eventID string) error
}

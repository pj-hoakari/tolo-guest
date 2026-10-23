package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

var (
	ErrGuestStatusNotFound  = errors.New("guest status not found")
	ErrDisplayNamesNotFound = errors.New("display names not found")
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
}

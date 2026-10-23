package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

var ErrGuestSettingsNotFound = errors.New("guest settings not found")

type GuestSettingsRepository interface {
	Find(ctx context.Context, eventID string) (domain.GuestSettings, error)
	Save(ctx context.Context, settings domain.GuestSettings) error
	ListEventTenants(ctx context.Context) ([]domain.EventTenant, error)
}

package application

//go:generate go tool mockgen -source=../repository/guest_settings.go -destination=mock_guest_settings_repository_test.go -package=application_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
	"github.com/pj-hoakari/tolo-guest/internal/tenantctx"
)

type UpdateGuestSettingsInput struct {
	EventID          string
	PageCompositions []domain.PageComposition
}

type GuestSettingsUseCases interface {
	GetGuestSettings(ctx context.Context, eventID string) (domain.GuestSettings, error)
	UpdateGuestSettings(ctx context.Context, input UpdateGuestSettingsInput) (domain.GuestSettings, error)
	ListIssuedURLs(ctx context.Context, eventID string) ([]domain.IssuedURL, error)
}

type GuestSettingsService struct {
	settings  repository.GuestSettingsRepository
	readModel repository.GuestReadModelRepository
}

func NewGuestSettingsService(
	settings repository.GuestSettingsRepository,
	readModel repository.GuestReadModelRepository,
) *GuestSettingsService {
	return &GuestSettingsService{settings: settings, readModel: readModel}
}

func (s *GuestSettingsService) GetGuestSettings(ctx context.Context, eventID string) (domain.GuestSettings, error) {
	if err := tenantctx.EnsureEvent(ctx, eventID); err != nil {
		return domain.GuestSettings{}, err
	}

	return findGuestSettings(ctx, s.settings, eventID)
}

func (s *GuestSettingsService) UpdateGuestSettings(
	ctx context.Context,
	input UpdateGuestSettingsInput,
) (domain.GuestSettings, error) {
	if err := tenantctx.EnsureEvent(ctx, input.EventID); err != nil {
		return domain.GuestSettings{}, err
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.GuestSettings{}, tenantctx.ErrMissing
	}

	settings, err := domain.NewGuestSettings(input.EventID, tenantPublicID, input.PageCompositions)
	if err != nil {
		return domain.GuestSettings{}, err
	}

	if err := s.settings.Save(ctx, settings); err != nil {
		return domain.GuestSettings{}, fmt.Errorf("save guest settings: %w", err)
	}

	return settings, nil
}

func (s *GuestSettingsService) ListIssuedURLs(ctx context.Context, eventID string) ([]domain.IssuedURL, error) {
	if err := tenantctx.EnsureEvent(ctx, eventID); err != nil {
		return nil, err
	}

	locations, err := s.readModel.FindQrLocations(ctx, eventID)
	if errors.Is(err, repository.ErrQrLocationsNotFound) {
		return []domain.IssuedURL{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("find QR locations: %w", err)
	}

	return locations.IssuedURLs(), nil
}

func findGuestSettings(
	ctx context.Context,
	settings repository.GuestSettingsRepository,
	eventID string,
) (domain.GuestSettings, error) {
	found, err := settings.Find(ctx, eventID)
	if errors.Is(err, repository.ErrGuestSettingsNotFound) {
		return domain.DefaultGuestSettings(eventID), nil
	}

	if err != nil {
		return domain.GuestSettings{}, fmt.Errorf("find guest settings: %w", err)
	}

	return found, nil
}

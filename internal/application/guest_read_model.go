package application

//go:generate go tool mockgen -source=../repository/guest_read_model.go -destination=mock_guest_read_model_repository_test.go -package=application_test
//go:generate go tool mockgen -source=guest_read_model.go -destination=mock_guest_read_model_sources_test.go -package=application_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

type GuestStatusSource interface {
	GetGuestSnapshot(ctx context.Context, eventID string) (domain.GuestStatus, bool, error)
}

type GuestMessageSource interface {
	ListGuestMessages(ctx context.Context, tenantPublicID, eventID string) ([]domain.GuestMessage, error)
}

type DisplayNameSource interface {
	GetDisplayNames(ctx context.Context, eventID string) (domain.DisplayNames, bool, error)
}

const (
	displayNameRequestBuffer = 64
	displayNameRefreshPause  = time.Minute
)

type GuestReadModelService struct {
	readModel    repository.GuestReadModelRepository
	settings     repository.GuestSettingsRepository
	statuses     GuestStatusSource
	messages     GuestMessageSource
	displayNames DisplayNameSource
	unknownNames chan string
}

func NewGuestReadModelService(
	readModel repository.GuestReadModelRepository,
	settings repository.GuestSettingsRepository,
	statuses GuestStatusSource,
	messages GuestMessageSource,
	displayNames DisplayNameSource,
) *GuestReadModelService {
	return &GuestReadModelService{
		readModel:    readModel,
		settings:     settings,
		statuses:     statuses,
		messages:     messages,
		displayNames: displayNames,
		unknownNames: make(chan string, displayNameRequestBuffer),
	}
}

func (s *GuestReadModelService) ApplyGuestStatus(ctx context.Context, status domain.GuestStatus) error {
	if err := s.readModel.ApplyStatus(ctx, status); err != nil {
		return fmt.Errorf("apply guest status: %w", err)
	}

	return nil
}

func (s *GuestReadModelService) ApplyGuestMessages(ctx context.Context, update domain.GuestMessagesUpdate) error {
	if err := s.readModel.ApplyMessages(ctx, update); err != nil {
		return fmt.Errorf("apply guest messages: %w", err)
	}

	return nil
}

func (s *GuestReadModelService) Reconcile(ctx context.Context) error {
	eventIDs, err := s.readModel.ListEventIDs(ctx)
	if err != nil {
		return fmt.Errorf("list read model events: %w", err)
	}

	tenants, err := s.settings.ListEventTenants(ctx)
	if err != nil {
		return fmt.Errorf("list event tenants: %w", err)
	}

	events := make(map[string]struct{}, len(eventIDs)+len(tenants))
	for _, eventID := range eventIDs {
		events[eventID] = struct{}{}
	}

	for _, tenant := range tenants {
		events[tenant.EventID] = struct{}{}
	}

	var errs []error

	for eventID := range events {
		if err := s.reconcileStatus(ctx, eventID); err != nil {
			errs = append(errs, fmt.Errorf("reconcile guest status of event %s: %w", eventID, err))
		}

		if err := s.reconcileDisplayNames(ctx, eventID); err != nil {
			errs = append(errs, fmt.Errorf("reconcile display names of event %s: %w", eventID, err))
		}
	}

	for _, tenant := range tenants {
		if err := s.reconcileMessages(ctx, tenant); err != nil {
			errs = append(errs, fmt.Errorf("reconcile guest messages of event %s: %w", tenant.EventID, err))
		}
	}

	return errors.Join(errs...)
}

func (s *GuestReadModelService) RunReconciliation(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	refreshedAt := map[string]time.Time{}

	s.reconcileOrLog(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileOrLog(ctx)
		case eventID := <-s.unknownNames:
			if time.Since(refreshedAt[eventID]) < displayNameRefreshPause {
				continue
			}

			refreshedAt[eventID] = time.Now()

			if err := s.reconcileDisplayNames(ctx, eventID); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "refresh display names failed", "event_id", eventID, "error", err)
			}
		}
	}
}

func (s *GuestReadModelService) reconcileOrLog(ctx context.Context) {
	if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "reconcile guest read model failed", "error", err)
	}
}

func (s *GuestReadModelService) requestDisplayNames(eventID string) {
	select {
	case s.unknownNames <- eventID:
	default:
	}
}

func (s *GuestReadModelService) reconcileDisplayNames(ctx context.Context, eventID string) error {
	names, found, err := s.displayNames.GetDisplayNames(ctx, eventID)
	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	return s.readModel.SaveDisplayNames(ctx, names)
}

func (s *GuestReadModelService) reconcileStatus(ctx context.Context, eventID string) error {
	status, found, err := s.statuses.GetGuestSnapshot(ctx, eventID)
	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	return s.readModel.ApplyStatus(ctx, status)
}

func (s *GuestReadModelService) reconcileMessages(ctx context.Context, tenant domain.EventTenant) error {
	messages, err := s.messages.ListGuestMessages(ctx, tenant.TenantPublicID, tenant.EventID)
	if err != nil {
		return err
	}

	return s.readModel.ReplaceMessages(ctx, tenant.EventID, messages)
}

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

type GuestReadModelService struct {
	readModel repository.GuestReadModelRepository
	settings  repository.GuestSettingsRepository
	statuses  GuestStatusSource
	messages  GuestMessageSource
}

func NewGuestReadModelService(
	readModel repository.GuestReadModelRepository,
	settings repository.GuestSettingsRepository,
	statuses GuestStatusSource,
	messages GuestMessageSource,
) *GuestReadModelService {
	return &GuestReadModelService{readModel: readModel, settings: settings, statuses: statuses, messages: messages}
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

	for {
		if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "reconcile guest read model failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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

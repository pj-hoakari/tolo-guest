package application

//go:generate go tool mockgen -source=../repository/guest_read_model.go -destination=mock_guest_read_model_repository_test.go -package=application_test
//go:generate go tool mockgen -source=../repository/qr_accesses.go -destination=mock_qr_access_repository_test.go -package=application_test
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

type QrLocationSource interface {
	GetQrLocations(ctx context.Context, tenantPublicID, eventID, revisionID string) (domain.QrLocations, bool, error)
}

type GuideMapSource interface {
	GetGuideMap(ctx context.Context, tenantPublicID, eventID, revisionID string) (*domain.GuideMap, bool, error)
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
	qrLocations  QrLocationSource
	guideMaps    GuideMapSource
	qrAccesses   repository.QrAccessRepository
	qrOpens      *QrOpenLimiter
	unknownNames chan string
}

func NewGuestReadModelService(
	readModel repository.GuestReadModelRepository,
	settings repository.GuestSettingsRepository,
	statuses GuestStatusSource,
	messages GuestMessageSource,
	displayNames DisplayNameSource,
	qrLocations QrLocationSource,
	guideMaps GuideMapSource,
	qrAccesses repository.QrAccessRepository,
	qrOpens *QrOpenLimiter,
) *GuestReadModelService {
	return &GuestReadModelService{
		readModel:    readModel,
		settings:     settings,
		statuses:     statuses,
		messages:     messages,
		displayNames: displayNames,
		qrLocations:  qrLocations,
		guideMaps:    guideMaps,
		qrAccesses:   qrAccesses,
		qrOpens:      qrOpens,
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

	revisions := make(map[string]string, len(events))

	for eventID := range events {
		if err := s.reconcileStatus(ctx, eventID); err != nil {
			errs = append(errs, fmt.Errorf("reconcile guest status of event %s: %w", eventID, err))
		}

		revisionID, err := s.reconcileDisplayNames(ctx, eventID)
		if err != nil {
			errs = append(errs, fmt.Errorf("reconcile display names of event %s: %w", eventID, err))
		}

		revisions[eventID] = revisionID
	}

	for _, tenant := range tenants {
		if err := s.reconcileMessages(ctx, tenant); err != nil {
			errs = append(errs, fmt.Errorf("reconcile guest messages of event %s: %w", tenant.EventID, err))
		}

		if err := s.reconcileQrLocations(ctx, tenant, revisions[tenant.EventID]); err != nil {
			errs = append(errs, fmt.Errorf("reconcile QR locations of event %s: %w", tenant.EventID, err))
		}

		if err := s.reconcileGuideMap(ctx, tenant, revisions[tenant.EventID]); err != nil {
			errs = append(errs, fmt.Errorf("reconcile guide map of event %s: %w", tenant.EventID, err))
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

			if _, err := s.reconcileDisplayNames(ctx, eventID); err != nil && ctx.Err() == nil {
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

func (s *GuestReadModelService) reconcileDisplayNames(ctx context.Context, eventID string) (string, error) {
	names, found, err := s.displayNames.GetDisplayNames(ctx, eventID)
	if err != nil {
		return "", err
	}

	if !found {
		return "", nil
	}

	if err := s.readModel.SaveDisplayNames(ctx, names); err != nil {
		return "", err
	}

	return names.RevisionID, nil
}

func (s *GuestReadModelService) reconcileQrLocations(ctx context.Context, tenant domain.EventTenant, revisionID string) error {
	if revisionID == "" {
		return nil
	}

	current, err := s.readModel.FindQrLocations(ctx, tenant.EventID)
	if err == nil && current.RevisionID == revisionID {
		return nil
	}

	if err != nil && !errors.Is(err, repository.ErrQrLocationsNotFound) {
		return err
	}

	locations, found, err := s.qrLocations.GetQrLocations(ctx, tenant.TenantPublicID, tenant.EventID, revisionID)
	if err != nil {
		return err
	}

	if !found {
		slog.WarnContext(ctx, "QR locations of the published revision are not found, keeping the previous ones",
			"event_id", tenant.EventID, "revision_id", revisionID)

		return nil
	}

	return s.readModel.SaveQrLocations(ctx, locations)
}

func (s *GuestReadModelService) reconcileGuideMap(ctx context.Context, tenant domain.EventTenant, revisionID string) error {
	if revisionID == "" {
		return nil
	}

	current, err := s.readModel.FindGuideMap(ctx, tenant.EventID)
	if err == nil && current.RevisionID == revisionID {
		return nil
	}

	if err != nil && !errors.Is(err, repository.ErrGuideMapNotFound) {
		return err
	}

	guideMap, found, err := s.guideMaps.GetGuideMap(ctx, tenant.TenantPublicID, tenant.EventID, revisionID)
	if err != nil {
		return err
	}

	if !found {
		slog.WarnContext(ctx, "guide map of the published revision is not found, keeping the previous one",
			"event_id", tenant.EventID, "revision_id", revisionID)

		return nil
	}

	if guideMap == nil {
		return s.readModel.DeleteGuideMap(ctx, tenant.EventID)
	}

	return s.readModel.SaveGuideMap(ctx, *guideMap)
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

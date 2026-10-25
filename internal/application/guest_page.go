package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

type GuestPageUseCases interface {
	OpenGuestPage(ctx context.Context, eventID, qrLocationID, sessionID string) ([]domain.Component, error)
	GetCrowd(ctx context.Context, eventID, language string) (domain.CrowdView, error)
	GetQueues(ctx context.Context, eventID, language string) (domain.QueuesView, error)
	ListGuestMessages(ctx context.Context, eventID string) ([]domain.GuestMessage, error)
	GetGuideMap(ctx context.Context, eventID, qrLocationID string) (GuideMapView, error)
}

type GuideMapView struct {
	GuideMap       domain.GuideMap
	CurrentPointID string
}

func (s *GuestReadModelService) OpenGuestPage(
	ctx context.Context,
	eventID, qrLocationID, sessionID string,
) ([]domain.Component, error) {
	if eventID == "" {
		return nil, domain.ErrEventIDRequired
	}

	if qrLocationID == "" {
		return nil, domain.ErrQrLocationIDRequired
	}

	locations, err := s.readModel.FindQrLocations(ctx, eventID)
	if errors.Is(err, repository.ErrQrLocationsNotFound) {
		return domain.DefaultComponents(), nil
	}

	if err != nil {
		return nil, fmt.Errorf("find QR locations: %w", err)
	}

	location, found := locations.Find(qrLocationID)
	if !found {
		return domain.DefaultComponents(), nil
	}

	settings, err := findGuestSettings(ctx, s.settings, eventID)
	if err != nil {
		return nil, err
	}

	if settings.TenantPublicID != "" && s.qrOpens.Allow(eventID, qrLocationID, sessionID) {
		access := domain.QrAccess{EventID: eventID, QrLocationID: qrLocationID, TenantPublicID: settings.TenantPublicID}
		if err := s.qrAccesses.RecordQrAccess(ctx, access); err != nil {
			slog.ErrorContext(ctx, "record QR access failed", "event_id", eventID, "qr_location_id", qrLocationID, "error", err)
		}
	}

	return settings.Components(location.Kind), nil
}

func (s *GuestReadModelService) GetCrowd(ctx context.Context, eventID, language string) (domain.CrowdView, error) {
	status, names, err := s.findNamedStatus(ctx, eventID)
	if err != nil {
		return domain.CrowdView{}, err
	}

	view, complete := names.NameCrowd(status, language)
	if !complete {
		s.requestDisplayNames(eventID)
	}

	return view, nil
}

func (s *GuestReadModelService) GetQueues(ctx context.Context, eventID, language string) (domain.QueuesView, error) {
	status, names, err := s.findNamedStatus(ctx, eventID)
	if err != nil {
		return domain.QueuesView{}, err
	}

	view, complete := names.NameQueues(status, language)
	if !complete {
		s.requestDisplayNames(eventID)
	}

	return view, nil
}

func (s *GuestReadModelService) ListGuestMessages(ctx context.Context, eventID string) ([]domain.GuestMessage, error) {
	if eventID == "" {
		return nil, domain.ErrEventIDRequired
	}

	messages, err := s.readModel.ListMessages(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("list guest messages: %w", err)
	}

	return messages, nil
}

func (s *GuestReadModelService) GetGuideMap(ctx context.Context, eventID, qrLocationID string) (GuideMapView, error) {
	if eventID == "" {
		return GuideMapView{}, domain.ErrEventIDRequired
	}

	guideMap, err := s.readModel.FindGuideMap(ctx, eventID)
	if err != nil {
		return GuideMapView{}, fmt.Errorf("find guide map: %w", err)
	}

	view := GuideMapView{GuideMap: guideMap, CurrentPointID: ""}

	locations, err := s.readModel.FindQrLocations(ctx, eventID)
	if errors.Is(err, repository.ErrQrLocationsNotFound) {
		return view, nil
	}

	if err != nil {
		return GuideMapView{}, fmt.Errorf("find QR locations: %w", err)
	}

	if location, found := locations.Find(qrLocationID); found && qrLocationID != "" {
		view.CurrentPointID = guideMap.CurrentPointID(location)
	}

	return view, nil
}

func (s *GuestReadModelService) findNamedStatus(
	ctx context.Context,
	eventID string,
) (domain.GuestStatus, domain.DisplayNames, error) {
	none := domain.DisplayNames{EventID: eventID, RevisionID: "", Points: nil, Routes: nil, RouteEndpoints: nil}

	if eventID == "" {
		return domain.GuestStatus{}, none, domain.ErrEventIDRequired
	}

	status, err := s.readModel.FindStatus(ctx, eventID)
	if errors.Is(err, repository.ErrGuestStatusNotFound) {
		return domain.GuestStatus{EventID: eventID, Sequence: 0, TakenAt: time.Time{}, Crowd: nil, Queues: nil}, none, nil
	}

	if err != nil {
		return domain.GuestStatus{}, none, fmt.Errorf("find guest status: %w", err)
	}

	names, err := s.readModel.FindDisplayNames(ctx, eventID)
	if errors.Is(err, repository.ErrDisplayNamesNotFound) {
		return status, none, nil
	}

	if err != nil {
		return domain.GuestStatus{}, none, fmt.Errorf("find display names: %w", err)
	}

	return status, names, nil
}

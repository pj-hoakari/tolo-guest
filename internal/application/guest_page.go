package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

type GuestPageUseCases interface {
	GetCrowd(ctx context.Context, eventID, language string) (domain.CrowdView, error)
	GetQueues(ctx context.Context, eventID, language string) (domain.QueuesView, error)
	ListGuestMessages(ctx context.Context, eventID string) ([]domain.GuestMessage, error)
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

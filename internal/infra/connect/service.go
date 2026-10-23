package connect

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	guestv1 "github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1"
	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

var protoComponents = map[domain.Component]guestv1.GuestComponent{
	domain.ComponentCrowd:         guestv1.GuestComponent_GUEST_COMPONENT_CROWD,
	domain.ComponentQueues:        guestv1.GuestComponent_GUEST_COMPONENT_QUEUES,
	domain.ComponentStaffMessages: guestv1.GuestComponent_GUEST_COMPONENT_STAFF_MESSAGES,
}

var protoCrowdLevels = map[domain.CrowdLevel]guestv1.CrowdLevel{
	domain.CrowdLevelUnspecified: guestv1.CrowdLevel_CROWD_LEVEL_UNSPECIFIED,
	domain.CrowdLevelLow:         guestv1.CrowdLevel_CROWD_LEVEL_LOW,
	domain.CrowdLevelMid:         guestv1.CrowdLevel_CROWD_LEVEL_MID,
	domain.CrowdLevelHigh:        guestv1.CrowdLevel_CROWD_LEVEL_HIGH,
}

type GuestPageService struct {
	guestv1connect.UnimplementedGuestPageServiceHandler

	useCases application.GuestPageUseCases
}

func NewGuestPageService(useCases application.GuestPageUseCases) *GuestPageService {
	return &GuestPageService{
		UnimplementedGuestPageServiceHandler: guestv1connect.UnimplementedGuestPageServiceHandler{},
		useCases:                             useCases,
	}
}

func (s *GuestPageService) GetCrowd(ctx context.Context, req *guestv1.GetCrowdRequest) (*guestv1.GetCrowdResponse, error) {
	view, err := s.useCases.GetCrowd(ctx, req.GetEventId(), req.GetLanguage())
	if err != nil {
		return nil, connectError(ctx, err)
	}

	crowd := make([]*guestv1.Crowd, 0, len(view.Crowd))
	for _, c := range view.Crowd {
		entry := &guestv1.Crowd{Target: nil, DisplayName: c.DisplayName, Level: protoCrowdLevels[c.Level], Danger: c.Danger}

		switch c.Target.Kind {
		case domain.CrowdTargetPoint:
			entry.Target = &guestv1.Crowd_PointId{PointId: c.Target.ID}
		case domain.CrowdTargetRoute:
			entry.Target = &guestv1.Crowd_RouteId{RouteId: c.Target.ID}
		case domain.CrowdTargetUnspecified:
		}

		crowd = append(crowd, entry)
	}

	return &guestv1.GetCrowdResponse{TakenAt: protoTakenAt(view.TakenAt), Crowd: crowd}, nil
}

func (s *GuestPageService) GetQueues(ctx context.Context, req *guestv1.GetQueuesRequest) (*guestv1.GetQueuesResponse, error) {
	view, err := s.useCases.GetQueues(ctx, req.GetEventId(), req.GetLanguage())
	if err != nil {
		return nil, connectError(ctx, err)
	}

	queues := make([]*guestv1.Queue, 0, len(view.Queues))
	for _, q := range view.Queues {
		gates := make([]*guestv1.QueueGate, 0, len(q.Gates))
		for _, g := range q.Gates {
			gates = append(gates, &guestv1.QueueGate{PointId: g.PointID, DisplayName: g.DisplayName, Recommended: g.Recommended})
		}

		queues = append(queues, &guestv1.Queue{
			QueueId:         q.QueueID,
			DisplayName:     q.DisplayName,
			WaitTimeMinutes: q.WaitTimeMinutes,
			TurnaroundCount: q.TurnaroundCount,
			Gates:           gates,
		})
	}

	return &guestv1.GetQueuesResponse{TakenAt: protoTakenAt(view.TakenAt), Queues: queues}, nil
}

func (s *GuestPageService) ListGuestMessages(
	ctx context.Context,
	req *guestv1.ListGuestMessagesRequest,
) (*guestv1.ListGuestMessagesResponse, error) {
	messages, err := s.useCases.ListGuestMessages(ctx, req.GetEventId())
	if err != nil {
		return nil, connectError(ctx, err)
	}

	protoMessages := make([]*guestv1.GuestMessage, 0, len(messages))
	for _, m := range messages {
		protoMessages = append(protoMessages, &guestv1.GuestMessage{
			MessageId: m.MessageID,
			Body:      m.Body,
			SentAt:    timestamppb.New(m.SentAt),
		})
	}

	return &guestv1.ListGuestMessagesResponse{Messages: protoMessages}, nil
}

func protoTakenAt(takenAt time.Time) *timestamppb.Timestamp {
	if takenAt.IsZero() {
		return nil
	}

	return timestamppb.New(takenAt)
}

type GuestSettingsService struct {
	guestv1connect.UnimplementedGuestSettingsServiceHandler

	useCases application.GuestSettingsUseCases
}

func NewGuestSettingsService(useCases application.GuestSettingsUseCases) *GuestSettingsService {
	return &GuestSettingsService{
		UnimplementedGuestSettingsServiceHandler: guestv1connect.UnimplementedGuestSettingsServiceHandler{},
		useCases:                                 useCases,
	}
}

func (s *GuestSettingsService) GetGuestSettings(
	ctx context.Context,
	req *guestv1.GetGuestSettingsRequest,
) (*guestv1.GuestSettings, error) {
	settings, err := s.useCases.GetGuestSettings(ctx, req.GetEventId())
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return newProtoGuestSettings(settings), nil
}

func (s *GuestSettingsService) UpdateGuestSettings(
	ctx context.Context,
	req *guestv1.UpdateGuestSettingsRequest,
) (*guestv1.GuestSettings, error) {
	compositions := make([]domain.PageComposition, 0, len(req.GetPageCompositions()))

	for _, composition := range req.GetPageCompositions() {
		components := make([]domain.Component, 0, len(composition.GetComponents()))
		for _, component := range composition.GetComponents() {
			components = append(components, domainComponent(component))
		}

		compositions = append(compositions, domain.PageComposition{Kind: composition.GetKind(), Components: components})
	}

	settings, err := s.useCases.UpdateGuestSettings(ctx, application.UpdateGuestSettingsInput{
		EventID:          req.GetEventId(),
		PageCompositions: compositions,
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return newProtoGuestSettings(settings), nil
}

func (s *GuestSettingsService) ListIssuedUrls(
	ctx context.Context,
	req *guestv1.ListIssuedUrlsRequest,
) (*guestv1.ListIssuedUrlsResponse, error) {
	issued, err := s.useCases.ListIssuedURLs(ctx, req.GetEventId())
	if err != nil {
		return nil, connectError(ctx, err)
	}

	urls := make([]*guestv1.IssuedUrl, 0, len(issued))
	for _, u := range issued {
		urls = append(urls, &guestv1.IssuedUrl{QrLocationId: u.QrLocationID, Name: u.Name, Kind: u.Kind, Path: u.Path})
	}

	return &guestv1.ListIssuedUrlsResponse{IssuedUrls: urls}, nil
}

func newProtoGuestSettings(settings domain.GuestSettings) *guestv1.GuestSettings {
	compositions := make([]*guestv1.PageComposition, 0, len(settings.PageCompositions))

	for _, composition := range settings.PageCompositions {
		components := make([]guestv1.GuestComponent, 0, len(composition.Components))
		for _, component := range composition.Components {
			components = append(components, protoComponents[component])
		}

		compositions = append(compositions, &guestv1.PageComposition{Kind: composition.Kind, Components: components})
	}

	return &guestv1.GuestSettings{EventId: settings.EventID, PageCompositions: compositions}
}

func domainComponent(component guestv1.GuestComponent) domain.Component {
	for domainComponent, protoComponent := range protoComponents {
		if protoComponent == component {
			return domainComponent
		}
	}

	return domain.ComponentUnspecified
}

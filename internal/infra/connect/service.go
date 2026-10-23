package connect

import (
	"context"

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

type GuestPageService struct {
	guestv1connect.UnimplementedGuestPageServiceHandler
}

func NewGuestPageService() *GuestPageService {
	return &GuestPageService{
		UnimplementedGuestPageServiceHandler: guestv1connect.UnimplementedGuestPageServiceHandler{},
	}
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

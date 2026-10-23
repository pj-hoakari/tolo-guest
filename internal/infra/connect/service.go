package connect

import (
	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
)

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
}

func NewGuestSettingsService() *GuestSettingsService {
	return &GuestSettingsService{
		UnimplementedGuestSettingsServiceHandler: guestv1connect.UnimplementedGuestSettingsServiceHandler{},
	}
}

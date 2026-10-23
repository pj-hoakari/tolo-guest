package domain

import (
	"errors"
	"slices"
	"strings"
)

var (
	ErrKindRequired         = errors.New("page composition kind is required")
	ErrComponentsRequired   = errors.New("page composition components are required")
	ErrDuplicateKind        = errors.New("page composition kind is duplicated")
	ErrComponentUnspecified = errors.New("page composition component is unspecified")
	ErrDuplicateComponent   = errors.New("page composition component is duplicated")
)

type Component int

const (
	ComponentUnspecified Component = iota
	ComponentCrowd
	ComponentQueues
	ComponentStaffMessages
)

type PageComposition struct {
	Kind       string
	Components []Component
}

type GuestSettings struct {
	EventID          string
	TenantPublicID   string
	PageCompositions []PageComposition
}

type EventTenant struct {
	EventID        string
	TenantPublicID string
}

func DefaultComponents() []Component {
	return []Component{ComponentCrowd, ComponentQueues, ComponentStaffMessages}
}

func DefaultGuestSettings(eventID string) GuestSettings {
	return GuestSettings{EventID: eventID, TenantPublicID: "", PageCompositions: nil}
}

func NewGuestSettings(eventID, tenantPublicID string, compositions []PageComposition) (GuestSettings, error) {
	kinds := make(map[string]struct{}, len(compositions))

	for _, composition := range compositions {
		if strings.TrimSpace(composition.Kind) == "" {
			return GuestSettings{}, ErrKindRequired
		}

		if _, ok := kinds[composition.Kind]; ok {
			return GuestSettings{}, ErrDuplicateKind
		}

		kinds[composition.Kind] = struct{}{}

		if len(composition.Components) == 0 {
			return GuestSettings{}, ErrComponentsRequired
		}

		for i, component := range composition.Components {
			if component < ComponentCrowd || component > ComponentStaffMessages {
				return GuestSettings{}, ErrComponentUnspecified
			}

			if slices.Contains(composition.Components[:i], component) {
				return GuestSettings{}, ErrDuplicateComponent
			}
		}
	}

	return GuestSettings{EventID: eventID, TenantPublicID: tenantPublicID, PageCompositions: compositions}, nil
}

func (s GuestSettings) Components(kind string) []Component {
	for _, composition := range s.PageCompositions {
		if composition.Kind == kind {
			return composition.Components
		}
	}

	return DefaultComponents()
}

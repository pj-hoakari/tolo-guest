package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

func TestNewGuestSettings(t *testing.T) {
	t.Parallel()

	crowd := domain.ComponentCrowd

	tests := []struct {
		name         string
		compositions []domain.PageComposition
		wantErr      error
	}{
		{"no compositions", nil, nil},
		{"components in any order", []domain.PageComposition{
			{Kind: "exit", Components: []domain.Component{domain.ComponentStaffMessages, crowd}},
			{Kind: "entrance", Components: []domain.Component{domain.ComponentQueues}},
		}, nil},
		{"a kind without components", []domain.PageComposition{{Kind: "blank", Components: nil}}, domain.ErrComponentsRequired},
		{"empty kind", []domain.PageComposition{{Kind: "", Components: []domain.Component{crowd}}}, domain.ErrKindRequired},
		{"blank kind", []domain.PageComposition{{Kind: " ", Components: []domain.Component{crowd}}}, domain.ErrKindRequired},
		{"duplicate kind", []domain.PageComposition{
			{Kind: "exit", Components: []domain.Component{crowd}},
			{Kind: "exit", Components: []domain.Component{crowd}},
		}, domain.ErrDuplicateKind},
		{"unspecified component", []domain.PageComposition{
			{Kind: "exit", Components: []domain.Component{domain.ComponentUnspecified}},
		}, domain.ErrComponentUnspecified},
		{"unknown component", []domain.PageComposition{
			{Kind: "exit", Components: []domain.Component{domain.Component(9)}},
		}, domain.ErrComponentUnspecified},
		{"duplicate component", []domain.PageComposition{
			{Kind: "exit", Components: []domain.Component{crowd, domain.ComponentQueues, crowd}},
		}, domain.ErrDuplicateComponent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.NewGuestSettings("event", "tenant", tt.compositions)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewGuestSettings() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGuestSettingsComponents(t *testing.T) {
	t.Parallel()

	everything := []domain.Component{domain.ComponentCrowd, domain.ComponentQueues, domain.ComponentStaffMessages}

	configured, err := domain.NewGuestSettings("event", "tenant", []domain.PageComposition{
		{Kind: "exit", Components: []domain.Component{domain.ComponentStaffMessages}},
	})
	if err != nil {
		t.Fatalf("NewGuestSettings() error = %v", err)
	}

	tests := []struct {
		name     string
		settings domain.GuestSettings
		kind     string
		want     []domain.Component
	}{
		{"configured kind", configured, "exit", []domain.Component{domain.ComponentStaffMessages}},
		{"kind missing from the settings", configured, "entrance", everything},
		{"event without settings", domain.DefaultGuestSettings("event"), "exit", everything},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.settings.Components(tt.kind); !slices.Equal(got, tt.want) {
				t.Errorf("Components(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

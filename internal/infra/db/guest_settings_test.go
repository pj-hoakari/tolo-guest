package db

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
	"github.com/pj-hoakari/tolo-guest/internal/tenantctx"
)

func TestGuestSettingsRepository(t *testing.T) {
	t.Parallel()

	repo := NewPostgresGuestSettingsRepository(testDB)
	ctx := context.Background()

	settings := domain.GuestSettings{
		EventID:        "guest-settings-event",
		TenantPublicID: "guest-settings-tenant",
		PageCompositions: []domain.PageComposition{
			{Kind: "exit", Components: []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd}},
			{Kind: "entrance", Components: []domain.Component{domain.ComponentQueues, domain.ComponentStaffMessages, domain.ComponentCrowd}},
			{Kind: "blank", Components: []domain.Component{}},
		},
	}

	t.Run("reports an event without settings as not found", func(t *testing.T) {
		if _, err := repo.Find(ctx, "guest-settings-unknown"); !errors.Is(err, repository.ErrGuestSettingsNotFound) {
			t.Errorf("Find() error = %v, want %v", err, repository.ErrGuestSettingsNotFound)
		}
	})

	t.Run("finds the saved settings with their order and tenant", func(t *testing.T) {
		if err := repo.Save(ctx, domain.GuestSettings{
			EventID:          settings.EventID,
			TenantPublicID:   settings.TenantPublicID,
			PageCompositions: []domain.PageComposition{{Kind: "gone", Components: []domain.Component{domain.ComponentCrowd}}},
		}); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		if err := repo.Save(ctx, settings); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		got, err := repo.Find(ctx, settings.EventID)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}

		if !reflect.DeepEqual(got, settings) {
			t.Errorf("Find() = %+v, want %+v", got, settings)
		}
	})

	t.Run("refuses to move an event to another tenant", func(t *testing.T) {
		moved := settings
		moved.TenantPublicID = "guest-settings-other-tenant"
		moved.PageCompositions = nil

		if err := repo.Save(ctx, moved); !errors.Is(err, tenantctx.ErrMismatch) {
			t.Errorf("Save() error = %v, want %v", err, tenantctx.ErrMismatch)
		}

		foreign := internaljwt.ContextWithClaims(ctx, internaljwt.Claims{TenantPublicID: moved.TenantPublicID})
		if _, err := repo.Find(foreign, settings.EventID); !errors.Is(err, tenantctx.ErrMismatch) {
			t.Errorf("Find() for another tenant error = %v, want %v", err, tenantctx.ErrMismatch)
		}

		got, err := repo.Find(ctx, settings.EventID)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}

		if !reflect.DeepEqual(got, settings) {
			t.Errorf("Find() after the refused save = %+v, want %+v", got, settings)
		}
	})

	t.Run("lists the tenant of every event with settings", func(t *testing.T) {
		if err := repo.Save(ctx, domain.GuestSettings{
			EventID:          "guest-settings-second-event",
			TenantPublicID:   "guest-settings-second-tenant",
			PageCompositions: nil,
		}); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		got, err := repo.ListEventTenants(ctx)
		if err != nil {
			t.Fatalf("ListEventTenants() error = %v", err)
		}

		for _, want := range []domain.EventTenant{
			{EventID: settings.EventID, TenantPublicID: settings.TenantPublicID},
			{EventID: "guest-settings-second-event", TenantPublicID: "guest-settings-second-tenant"},
		} {
			if !slices.Contains(got, want) {
				t.Errorf("ListEventTenants() = %v, want it to contain %v", got, want)
			}
		}
	})
}

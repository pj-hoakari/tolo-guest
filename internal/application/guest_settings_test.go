package application_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
	"github.com/pj-hoakari/tolo-guest/internal/tenantctx"
)

const (
	testTenantPublicID = "a1b2c3d4e5f60718"
	testEventPublicID  = "0f1e2d3c4b5a6978"
	otherEventPublicID = "9876543210fedcba"
)

func withEventToken(eventPublicID string) context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  eventPublicID,
	})
}

func newGuestSettingsService(t *testing.T) (*application.GuestSettingsService, *MockGuestSettingsRepository) {
	t.Helper()

	settings := NewMockGuestSettingsRepository(gomock.NewController(t))

	return application.NewGuestSettingsService(settings), settings
}

func TestGetGuestSettings(t *testing.T) {
	t.Parallel()

	t.Run("answers the defaults for an event without stored settings", func(t *testing.T) {
		t.Parallel()

		service, settings := newGuestSettingsService(t)
		settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(domain.GuestSettings{}, repository.ErrGuestSettingsNotFound)

		got, err := service.GetGuestSettings(withEventToken(testEventPublicID), testEventPublicID)
		if err != nil {
			t.Fatalf("GetGuestSettings() error = %v", err)
		}

		if got.EventID != testEventPublicID || got.TenantPublicID != "" || len(got.PageCompositions) != 0 {
			t.Errorf("GetGuestSettings() = %+v, want the defaults of %s", got, testEventPublicID)
		}
	})

	t.Run("rejects a token for another event before reading", func(t *testing.T) {
		t.Parallel()

		service, _ := newGuestSettingsService(t)

		_, err := service.GetGuestSettings(withEventToken(otherEventPublicID), testEventPublicID)
		if !errors.Is(err, tenantctx.ErrEventMismatch) {
			t.Errorf("GetGuestSettings() error = %v, want %v", err, tenantctx.ErrEventMismatch)
		}
	})
}

func TestUpdateGuestSettings(t *testing.T) {
	t.Parallel()

	compositions := []domain.PageComposition{{Kind: "exit", Components: []domain.Component{domain.ComponentQueues}}}

	t.Run("saves the settings under the tenant of the token", func(t *testing.T) {
		t.Parallel()

		service, settings := newGuestSettingsService(t)
		want := domain.GuestSettings{EventID: testEventPublicID, TenantPublicID: testTenantPublicID, PageCompositions: compositions}
		settings.EXPECT().Save(gomock.Any(), want).Return(nil)

		got, err := service.UpdateGuestSettings(withEventToken(testEventPublicID), application.UpdateGuestSettingsInput{
			EventID:          testEventPublicID,
			PageCompositions: compositions,
		})
		if err != nil {
			t.Fatalf("UpdateGuestSettings() error = %v", err)
		}

		if got.TenantPublicID != testTenantPublicID {
			t.Errorf("UpdateGuestSettings() tenant = %q, want %q", got.TenantPublicID, testTenantPublicID)
		}
	})

	t.Run("rejects without saving", func(t *testing.T) {
		t.Parallel()

		withoutTenant := internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
			TokenUse:      internaljwt.TokenUseEventAccess,
			EventPublicID: testEventPublicID,
		})

		for name, tc := range map[string]struct {
			ctx          context.Context
			compositions []domain.PageComposition
			want         error
		}{
			"a token for another event": {withEventToken(otherEventPublicID), compositions, tenantctx.ErrEventMismatch},
			"a token without a tenant":  {withoutTenant, compositions, tenantctx.ErrMissing},
			"invalid compositions": {withEventToken(testEventPublicID), []domain.PageComposition{
				{Kind: "", Components: nil},
			}, domain.ErrKindRequired},
		} {
			service, _ := newGuestSettingsService(t)

			_, err := service.UpdateGuestSettings(tc.ctx, application.UpdateGuestSettingsInput{
				EventID:          testEventPublicID,
				PageCompositions: tc.compositions,
			})
			if !errors.Is(err, tc.want) {
				t.Errorf("%s: UpdateGuestSettings() error = %v, want %v", name, err, tc.want)
			}
		}
	})

	t.Run("reports a tenant conflict from the repository", func(t *testing.T) {
		t.Parallel()

		service, settings := newGuestSettingsService(t)
		settings.EXPECT().Save(gomock.Any(), gomock.Any()).Return(tenantctx.ErrMismatch)

		_, err := service.UpdateGuestSettings(withEventToken(testEventPublicID), application.UpdateGuestSettingsInput{
			EventID:          testEventPublicID,
			PageCompositions: compositions,
		})
		if !errors.Is(err, tenantctx.ErrMismatch) {
			t.Errorf("UpdateGuestSettings() error = %v, want %v", err, tenantctx.ErrMismatch)
		}
	})
}

package connect

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"

	guestv1 "github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1"
	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

const otherEventPublicID = "9876543210fedcba"

type memoryGuestSettings struct {
	mu      sync.Mutex
	byEvent map[string]domain.GuestSettings
}

func newGuestSettingsUseCases() *application.GuestSettingsService {
	return application.NewGuestSettingsService(&memoryGuestSettings{byEvent: map[string]domain.GuestSettings{}}, newMemoryReadModel())
}

func (m *memoryGuestSettings) Find(_ context.Context, eventID string) (domain.GuestSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	settings, ok := m.byEvent[eventID]
	if !ok {
		return domain.GuestSettings{}, repository.ErrGuestSettingsNotFound
	}

	return settings, nil
}

func (m *memoryGuestSettings) Save(_ context.Context, settings domain.GuestSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.byEvent[settings.EventID] = settings

	return nil
}

func (m *memoryGuestSettings) ListEventTenants(context.Context) ([]domain.EventTenant, error) {
	return nil, nil
}

func TestGuestSettings(t *testing.T) {
	t.Parallel()

	generator, err := jwtgen.NewGenerator("guest-settings-key")
	if err != nil {
		t.Fatalf("create token generator: %v", err)
	}

	keys, err := generator.JWKS()
	if err != nil {
		t.Fatalf("publish signing keys: %v", err)
	}

	client := guestv1connect.NewGuestSettingsServiceClient(newClient(newTestServer(t, newTestHandler(t, keys))))

	token := func(t *testing.T, eventPublicID, scope string) string {
		t.Helper()

		output, err := generator.Generate(jwtgen.Config{
			Issuer:         DefaultInternalJWTIssuer,
			Audience:       DefaultInternalJWTAudience,
			TokenUse:       internaljwt.TokenUseEventAccess,
			Subject:        testSubject,
			TenantPublicID: testTenantPublicID,
			EventPublicID:  eventPublicID,
			Scope:          scope,
			TTL:            time.Hour,
		})
		if err != nil {
			t.Fatalf("mint %s token: %v", scope, err)
		}

		return "Bearer " + output.Token
	}

	get := func(t *testing.T, eventID string) *guestv1.GuestSettings {
		t.Helper()

		res, err := client.GetGuestSettings(
			authorized(token(t, eventID, "events.read")),
			&guestv1.GetGuestSettingsRequest{EventId: eventID},
		)
		if err != nil {
			t.Fatalf("GetGuestSettings() error = %v", err)
		}

		return res
	}

	update := func(t *testing.T, eventID string, compositions []*guestv1.PageComposition) (*guestv1.GuestSettings, error) {
		t.Helper()

		return client.UpdateGuestSettings(
			authorized(token(t, eventID, "events.manage")),
			&guestv1.UpdateGuestSettingsRequest{EventId: eventID, PageCompositions: compositions},
		)
	}

	stored := []*guestv1.PageComposition{
		{Kind: "exit", Components: []guestv1.GuestComponent{
			guestv1.GuestComponent_GUEST_COMPONENT_STAFF_MESSAGES,
			guestv1.GuestComponent_GUEST_COMPONENT_CROWD,
		}},
		{Kind: "entrance", Components: []guestv1.GuestComponent{
			guestv1.GuestComponent_GUEST_COMPONENT_QUEUES,
		}},
	}

	assertCompositions := func(t *testing.T, label string, got *guestv1.GuestSettings, eventID string, want []*guestv1.PageComposition) {
		t.Helper()

		if got.GetEventId() != eventID {
			t.Errorf("%s event_id = %q, want %q", label, got.GetEventId(), eventID)
		}

		if !slices.EqualFunc(got.GetPageCompositions(), want, func(a, b *guestv1.PageComposition) bool {
			return a.GetKind() == b.GetKind() && slices.Equal(a.GetComponents(), b.GetComponents())
		}) {
			t.Errorf("%s page_compositions = %v, want %v", label, got.GetPageCompositions(), want)
		}
	}

	t.Run("answers no compositions for an event without stored settings", func(t *testing.T) {
		assertCompositions(t, "settings", get(t, otherEventPublicID), otherEventPublicID, nil)
	})

	t.Run("replaces the settings with what an update sends, in its order", func(t *testing.T) {
		if _, err := update(t, testEventPublicID, []*guestv1.PageComposition{
			{Kind: "gone", Components: []guestv1.GuestComponent{guestv1.GuestComponent_GUEST_COMPONENT_CROWD}},
		}); err != nil {
			t.Fatalf("UpdateGuestSettings() error = %v", err)
		}

		updated, err := update(t, testEventPublicID, stored)
		if err != nil {
			t.Fatalf("UpdateGuestSettings() error = %v", err)
		}

		assertCompositions(t, "updated", updated, testEventPublicID, stored)
		assertCompositions(t, "stored", get(t, testEventPublicID), testEventPublicID, stored)
	})

	t.Run("rejects invalid compositions and keeps the stored settings", func(t *testing.T) {
		crowd := guestv1.GuestComponent_GUEST_COMPONENT_CROWD

		for name, compositions := range map[string][]*guestv1.PageComposition{
			"empty kind":            {{Kind: "", Components: []guestv1.GuestComponent{crowd}}},
			"blank kind":            {{Kind: "  ", Components: []guestv1.GuestComponent{crowd}}},
			"unspecified component": {{Kind: "exit", Components: []guestv1.GuestComponent{guestv1.GuestComponent_GUEST_COMPONENT_UNSPECIFIED}}},
			"unknown component":     {{Kind: "exit", Components: []guestv1.GuestComponent{guestv1.GuestComponent(9)}}},
			"duplicate kind": {
				{Kind: "exit", Components: []guestv1.GuestComponent{crowd}},
				{Kind: "exit", Components: []guestv1.GuestComponent{guestv1.GuestComponent_GUEST_COMPONENT_QUEUES}},
			},
			"duplicate component": {{Kind: "exit", Components: []guestv1.GuestComponent{crowd, crowd}}},
		} {
			_, err := update(t, testEventPublicID, compositions)
			if got := connectrpc.CodeOf(err); got != connectrpc.CodeInvalidArgument {
				t.Errorf("%s: code = %v, want %v (error %v)", name, got, connectrpc.CodeInvalidArgument, err)
			}
		}

		assertCompositions(t, "stored", get(t, testEventPublicID), testEventPublicID, stored)
	})

	t.Run("rejects a token for another event", func(t *testing.T) {
		foreign := authorized(token(t, otherEventPublicID, "events.read events.manage"))

		_, getErr := client.GetGuestSettings(foreign, &guestv1.GetGuestSettingsRequest{EventId: testEventPublicID})
		_, updateErr := client.UpdateGuestSettings(foreign, &guestv1.UpdateGuestSettingsRequest{EventId: testEventPublicID})

		for name, err := range map[string]error{"Get": getErr, "Update": updateErr} {
			if got := connectrpc.CodeOf(err); got != connectrpc.CodePermissionDenied {
				t.Errorf("%s: code = %v, want %v (error %v)", name, got, connectrpc.CodePermissionDenied, err)
			}
		}

		assertCompositions(t, "stored", get(t, testEventPublicID), testEventPublicID, stored)
	})
}

func TestListIssuedUrls(t *testing.T) {
	t.Parallel()

	generator, err := jwtgen.NewGenerator("issued-urls-key")
	if err != nil {
		t.Fatalf("create token generator: %v", err)
	}

	keys, err := generator.JWKS()
	if err != nil {
		t.Fatalf("publish signing keys: %v", err)
	}

	readModel := newMemoryReadModel()
	_ = readModel.SaveQrLocations(context.Background(), domain.QrLocations{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Locations: []domain.QrLocation{
			{QrLocationID: "qr-west", Name: "西門", Kind: "entrance"},
			{QrLocationID: "qr/hall 1", Name: "ホール前", Kind: "queue"},
		},
	})

	routes, err := RoutesWithVerifier(
		newTestVerifier(t, keys),
		application.NewGuestSettingsService(&memoryGuestSettings{byEvent: map[string]domain.GuestSettings{}}, readModel),
		newGuestPageUseCases(readModel),
	)
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	client := guestv1connect.NewGuestSettingsServiceClient(newClient(newTestServer(t, mux)))

	list := func(t *testing.T, tokenEventID, scope, eventID string) (*guestv1.ListIssuedUrlsResponse, error) {
		t.Helper()

		output, err := generator.Generate(jwtgen.Config{
			Issuer:         DefaultInternalJWTIssuer,
			Audience:       DefaultInternalJWTAudience,
			TokenUse:       internaljwt.TokenUseEventAccess,
			Subject:        testSubject,
			TenantPublicID: testTenantPublicID,
			EventPublicID:  tokenEventID,
			Scope:          scope,
			TTL:            time.Hour,
		})
		if err != nil {
			t.Fatalf("mint %s token: %v", scope, err)
		}

		return client.ListIssuedUrls(authorized("Bearer "+output.Token), &guestv1.ListIssuedUrlsRequest{EventId: eventID})
	}

	t.Run("issues one percent-encoded path per QR location in the stored order", func(t *testing.T) {
		t.Parallel()

		got, err := list(t, testEventPublicID, "events.read", testEventPublicID)
		if err != nil {
			t.Fatalf("ListIssuedUrls() error = %v", err)
		}

		want := &guestv1.ListIssuedUrlsResponse{IssuedUrls: []*guestv1.IssuedUrl{
			{QrLocationId: "qr-west", Name: "西門", Kind: "entrance", Path: "/event/" + testEventPublicID + "/guest/qr-west"},
			{QrLocationId: "qr/hall 1", Name: "ホール前", Kind: "queue", Path: "/event/" + testEventPublicID + "/guest/qr%2Fhall%201"},
		}}
		if !proto.Equal(got, want) {
			t.Errorf("ListIssuedUrls() = %v, want %v", got, want)
		}
	})

	t.Run("answers no URLs for an event whose QR locations are not pulled yet", func(t *testing.T) {
		t.Parallel()

		got, err := list(t, otherEventPublicID, "events.read", otherEventPublicID)
		if err != nil || len(got.GetIssuedUrls()) != 0 {
			t.Errorf("ListIssuedUrls() = %v, %v, want no URLs without error", got, err)
		}
	})

	t.Run("rejects a token without events.read", func(t *testing.T) {
		t.Parallel()

		_, err := list(t, testEventPublicID, "events.manage", testEventPublicID)
		if got := connectrpc.CodeOf(err); got != connectrpc.CodePermissionDenied {
			t.Errorf("code = %v, want %v (error %v)", got, connectrpc.CodePermissionDenied, err)
		}
	})

	t.Run("rejects a token for another event", func(t *testing.T) {
		t.Parallel()

		_, err := list(t, otherEventPublicID, "events.read", testEventPublicID)
		if got := connectrpc.CodeOf(err); got != connectrpc.CodePermissionDenied {
			t.Errorf("code = %v, want %v (error %v)", got, connectrpc.CodePermissionDenied, err)
		}
	})
}

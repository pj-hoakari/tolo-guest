package connect

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwks"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"
	"github.com/pj-hoakari/internal-jwt-handling/verifier"

	guestv1 "github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1"
	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
)

// newTestJWKSURL serves keys from an httptest endpoint, mirroring the Service
// Gateway publishing its signing keys, and returns the URL to fetch them from.
func newTestJWKSURL(t *testing.T, keys internaljwt.JWKS) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(keys); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server.URL
}

// newTestVerifier builds a verifier backed by an httptest JWKS endpoint serving
// keys.
func newTestVerifier(t *testing.T, keys internaljwt.JWKS) *verifier.Verifier {
	t.Helper()

	// The cooldowns are collapsed so that a refresh on an unknown kid is never
	// held off within a test run.
	cache, err := jwks.New(jwks.Config{
		URL:             newTestJWKSURL(t, keys),
		RefreshCooldown: time.Nanosecond,
		FailureCooldown: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("create JWKS cache: %v", err)
	}

	tokenVerifier, err := verifier.New(DefaultInternalJWTIssuer, DefaultInternalJWTAudience, cache)
	if err != nil {
		t.Fatalf("create internal JWT verifier: %v", err)
	}

	return tokenVerifier
}

// newTestHandler builds a handler serving the production service routes wired
// to a verifier trusting keys.
func newTestHandler(t *testing.T, keys internaljwt.JWKS) http.Handler {
	t.Helper()

	routes, err := RoutesWithVerifier(newTestVerifier(t, keys), newGuestSettingsUseCases(), newGuestPageUseCases(newMemoryReadModel()))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return mux
}

func newTestHandlerForJWKSURL(t *testing.T, jwksURL string) http.Handler {
	t.Helper()

	cache, err := jwks.New(jwks.Config{
		URL:             jwksURL,
		RefreshCooldown: time.Nanosecond,
		FailureCooldown: time.Nanosecond,
		RetryBackoff:    []time.Duration{},
	})
	if err != nil {
		t.Fatalf("create JWKS cache: %v", err)
	}

	tokenVerifier, err := verifier.New(DefaultInternalJWTIssuer, DefaultInternalJWTAudience, cache)
	if err != nil {
		t.Fatalf("create internal JWT verifier: %v", err)
	}

	routes, err := RoutesWithVerifier(tokenVerifier, newGuestSettingsUseCases(), newGuestPageUseCases(newMemoryReadModel()))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return mux
}

const (
	testTenantPublicID = "a1b2c3d4e5f60718"
	testEventPublicID  = "0f1e2d3c4b5a6978"
	testSubject        = "staff-user-1"
)

func mintEventToken(t *testing.T, scope string) (string, internaljwt.JWKS) {
	t.Helper()

	return mintInternalJWTFor(t, jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        testSubject,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  testEventPublicID,
		Scope:          scope,
	})
}

// mintInternalJWTFor issues an internal JWT signed by a fresh key, returning
// the Authorization header value and the JWKS document publishing the key.
func mintInternalJWTFor(t *testing.T, config jwtgen.Config) (string, internaljwt.JWKS) {
	t.Helper()

	if config.Issuer == "" {
		config.Issuer = DefaultInternalJWTIssuer
	}

	if config.Audience == "" {
		config.Audience = DefaultInternalJWTAudience
	}

	config.KeyID = "test-key"
	config.TTL = time.Hour

	output, err := jwtgen.Generate(config)
	if err != nil {
		t.Fatalf("generate internal JWT: %v", err)
	}

	return "Bearer " + output.Token, output.JWKS
}

func authorized(authorization string) context.Context {
	ctx, info := connectrpc.NewClientContext(context.Background())
	if authorization != "" {
		info.RequestHeader().Set("Authorization", authorization)
	}

	return ctx
}

func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	return httpServer
}

func newClient(httpServer *httptest.Server) *connectrpc.Client {
	return connectrpc.NewClient(connecthttp.NewTransport(httpServer.Client(), httpServer.URL))
}

func getGuestSettings(httpServer *httptest.Server, authorization string) error {
	client := guestv1connect.NewGuestSettingsServiceClient(newClient(httpServer))
	_, err := client.GetGuestSettings(authorized(authorization), &guestv1.GetGuestSettingsRequest{EventId: testEventPublicID})

	return err
}

func TestRoutesWithJWTSettings(t *testing.T) {
	t.Parallel()

	t.Run("verifies a token against the JWKS the settings locate", func(t *testing.T) {
		t.Parallel()

		authorization, keys := mintEventToken(t, "events.read")

		settings := DefaultJWTSettings()
		settings.JWKSURL = newTestJWKSURL(t, keys)

		routes, err := RoutesWithJWTSettings(settings, newGuestSettingsUseCases(), newGuestPageUseCases(newMemoryReadModel()))
		if err != nil {
			t.Fatalf("RoutesWithJWTSettings() error = %v", err)
		}

		mux := http.NewServeMux()
		routes(mux)

		if err := getGuestSettings(newTestServer(t, mux), authorization); err != nil {
			t.Fatalf("GetGuestSettings() error = %v", err)
		}
	})

	t.Run("rejects settings without a JWKS URL", func(t *testing.T) {
		t.Parallel()

		settings := DefaultJWTSettings()
		settings.JWKSURL = ""

		_, err := RoutesWithJWTSettings(settings, newGuestSettingsUseCases(), newGuestPageUseCases(newMemoryReadModel()))
		if !errors.Is(err, jwks.ErrMissingURL) {
			t.Fatalf("RoutesWithJWTSettings() error = %v, want %v", err, jwks.ErrMissingURL)
		}
	})
}

func TestEventAccessAuthz(t *testing.T) {
	t.Parallel()

	eventToken := jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        testSubject,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  testEventPublicID,
		Scope:          "events.read",
	}

	t.Run("accepts an event token with the required scope", func(t *testing.T) {
		t.Parallel()

		authorization, keys := mintInternalJWTFor(t, eventToken)

		if err := getGuestSettings(newTestServer(t, newTestHandler(t, keys)), authorization); err != nil {
			t.Fatalf("GetGuestSettings() error = %v", err)
		}
	})

	tests := []struct {
		name   string
		config func(jwtgen.Config) jwtgen.Config
		want   connectrpc.Code
	}{
		{"rejects an event token without the required scope", func(c jwtgen.Config) jwtgen.Config {
			c.Scope = "events.manage"

			return c
		}, connectrpc.CodePermissionDenied},
		{"rejects a tenant token", func(c jwtgen.Config) jwtgen.Config {
			c.TokenUse = internaljwt.TokenUseTenantAccess
			c.EventPublicID = ""

			return c
		}, connectrpc.CodeUnauthenticated},
		{"rejects a service token", func(c jwtgen.Config) jwtgen.Config {
			return jwtgen.Config{TokenUse: internaljwt.TokenUseService, Subject: "tolo-observation"}
		}, connectrpc.CodeUnauthenticated},
		{"rejects another audience", func(c jwtgen.Config) jwtgen.Config {
			c.Audience = "other-service"

			return c
		}, connectrpc.CodeUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			authorization, keys := mintInternalJWTFor(t, tt.config(eventToken))

			err := getGuestSettings(newTestServer(t, newTestHandler(t, keys)), authorization)
			if got := connectrpc.CodeOf(err); got != tt.want {
				t.Fatalf("GetGuestSettings() error code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthzRejectsMissingBearerToken(t *testing.T) {
	t.Parallel()

	_, keys := mintEventToken(t, "events.read")

	err := getGuestSettings(newTestServer(t, newTestHandler(t, keys)), "")
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("GetGuestSettings() error code = %v, want %v", got, want)
	}
}

func TestAuthzRejectsUnknownSigningKey(t *testing.T) {
	t.Parallel()

	// The handler trusts a JWKS publishing neither the key nor the kid that
	// signed the token below.
	_, trustedKeys := mintEventToken(t, "events.read")
	for i := range trustedKeys.Keys {
		trustedKeys.Keys[i].KeyID = "other-key"
	}

	foreignAuthorization, _ := mintEventToken(t, "events.read")

	err := getGuestSettings(newTestServer(t, newTestHandler(t, trustedKeys)), foreignAuthorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("GetGuestSettings() error code = %v, want %v", got, want)
	}
}

func TestAuthzUnavailableWhenJWKSUnreachable(t *testing.T) {
	t.Parallel()

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(jwksServer.Close)

	authorization, _ := mintEventToken(t, "events.read")

	err := getGuestSettings(newTestServer(t, newTestHandlerForJWKSURL(t, jwksServer.URL)), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnavailable; got != want {
		t.Fatalf("GetGuestSettings() error code = %v, want %v", got, want)
	}
}

func TestPublicProceduresIgnoreTheAuthorizationHeader(t *testing.T) {
	t.Parallel()

	_, keys := mintEventToken(t, "events.read")
	client := guestv1connect.NewGuestPageServiceClient(newClient(newTestServer(t, newTestHandler(t, keys))))

	calls := map[string]func(context.Context) error{
		"OpenGuestPage": func(ctx context.Context) error {
			_, err := client.OpenGuestPage(ctx, &guestv1.OpenGuestPageRequest{EventId: testEventPublicID, QrLocationId: "qr-east"})

			return err
		},
		"GetCrowd": func(ctx context.Context) error {
			_, err := client.GetCrowd(ctx, &guestv1.GetCrowdRequest{EventId: testEventPublicID})

			return err
		},
		"GetQueues": func(ctx context.Context) error {
			_, err := client.GetQueues(ctx, &guestv1.GetQueuesRequest{EventId: testEventPublicID})

			return err
		},
		"ListGuestMessages": func(ctx context.Context) error {
			_, err := client.ListGuestMessages(ctx, &guestv1.ListGuestMessagesRequest{EventId: testEventPublicID})

			return err
		},
	}

	for name, call := range calls {
		for _, authorization := range []string{"", "Bearer not-a-token"} {
			if err := call(authorized(authorization)); err != nil {
				t.Errorf("%s with Authorization %q error = %v, want none", name, authorization, err)
			}
		}
	}
}

package connect

import (
	"slices"
	"testing"

	"github.com/pj-hoakari/protoc-gen-authz-go/authz"

	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
)

func TestGeneratedPoliciesMatchTheSpecification(t *testing.T) {
	t.Parallel()

	eventAccess := func(scope string) authz.Policy {
		return authz.Policy{Level: authz.LevelAuthenticated, TokenUses: []string{"event_access"}, RequiredScopes: []string{scope}}
	}
	public := authz.Policy{Level: authz.LevelPublic}

	want := map[string]authz.Policy{
		guestv1connect.GuestPageServiceOpenGuestPageProcedure:           public,
		guestv1connect.GuestPageServiceGetCrowdProcedure:                public,
		guestv1connect.GuestPageServiceGetQueuesProcedure:               public,
		guestv1connect.GuestPageServiceListGuestMessagesProcedure:       public,
		guestv1connect.GuestSettingsServiceGetGuestSettingsProcedure:    eventAccess("events.read"),
		guestv1connect.GuestSettingsServiceUpdateGuestSettingsProcedure: eventAccess("events.manage"),
	}

	got, err := authz.Merge(
		guestv1connect.GuestPageServicePolicies,
		guestv1connect.GuestSettingsServicePolicies,
	)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	for procedure, wantPolicy := range want {
		policy, ok := got.Lookup(procedure)
		if !ok {
			t.Errorf("%s is not named by the generated policies", procedure)

			continue
		}

		if policy.Level != wantPolicy.Level {
			t.Errorf("%s level = %v, want %v", procedure, policy.Level, wantPolicy.Level)
		}

		if !slices.Equal(policy.TokenUses, wantPolicy.TokenUses) {
			t.Errorf("%s token uses = %v, want %v", procedure, policy.TokenUses, wantPolicy.TokenUses)
		}

		if !slices.Equal(policy.RequiredScopes, wantPolicy.RequiredScopes) {
			t.Errorf("%s required scopes = %v, want %v", procedure, policy.RequiredScopes, wantPolicy.RequiredScopes)
		}
	}

	for procedure := range got {
		if _, ok := want[procedure]; !ok {
			t.Errorf("%s is served without a declared policy", procedure)
		}
	}
}

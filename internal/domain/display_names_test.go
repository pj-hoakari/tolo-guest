package domain_test

import (
	"reflect"
	"testing"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

func TestLocalizedTextIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     domain.LocalizedText
		language string
		want     string
	}{
		{"the requested language", domain.LocalizedText{"ja": "東門", "en": "East Gate"}, "en", "East Gate"},
		{"the primary subtag of a regional language", domain.LocalizedText{"ja": "東門", "en": "East Gate"}, "en-US", "East Gate"},
		{"Japanese when the requested language is missing", domain.LocalizedText{"ja": "東門", "en": "East Gate"}, "fr", "東門"},
		{"Japanese when no language is requested", domain.LocalizedText{"en": "East Gate", "ja": "東門"}, "", "東門"},
		{"the first language code when Japanese is missing", domain.LocalizedText{"zh": "东门", "en": "East Gate"}, "fr", "East Gate"},
		{"nothing without labels", domain.LocalizedText{}, "ja", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.text.In(tt.language); got != tt.want {
				t.Errorf("In(%q) = %q, want %q", tt.language, got, tt.want)
			}
		})
	}
}

func testDisplayNames() domain.DisplayNames {
	return domain.DisplayNames{
		EventID:    "event",
		RevisionID: "rev-1",
		Points: map[string]domain.LocalizedText{
			"p-east":  {"ja": "東門", "en": "East Gate"},
			"p-west":  {"ja": "西門", "en": "West Gate"},
			"p-plaza": {"ja": "広場"},
		},
		Routes: map[string]string{"r-named": "メイン通り"},
		RouteEndpoints: map[string]domain.RouteEndpoints{
			"r-named":     {FromPointID: "p-east", ToPointID: "p-plaza"},
			"r-unnamed":   {FromPointID: "p-east", ToPointID: "p-west"},
			"r-unlabeled": {FromPointID: "p-east", ToPointID: "p-gone"},
		},
	}
}

func TestNameCrowd(t *testing.T) {
	t.Parallel()

	crowd := func(kind domain.CrowdTargetKind, id string) domain.CrowdStatus {
		return domain.CrowdStatus{Target: domain.CrowdTarget{Kind: kind, ID: id}, Level: domain.CrowdLevelMid, Danger: false}
	}

	tests := []struct {
		name         string
		target       domain.CrowdStatus
		wantName     string
		wantComplete bool
	}{
		{"a point in the requested language", crowd(domain.CrowdTargetPoint, "p-east"), "East Gate", true},
		{"a route by its display name", crowd(domain.CrowdTargetRoute, "r-named"), "メイン通り", true},
		{"a route without a display name from its endpoints", crowd(domain.CrowdTargetRoute, "r-unnamed"), "East Gate → West Gate", true},
		{"no route name when an endpoint has no label", crowd(domain.CrowdTargetRoute, "r-unlabeled"), "", true},
		{"an unknown point", crowd(domain.CrowdTargetPoint, "p-new"), "", false},
		{"an unknown route", crowd(domain.CrowdTargetRoute, "r-new"), "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			view, complete := testDisplayNames().NameCrowd(domain.GuestStatus{Crowd: []domain.CrowdStatus{tt.target}}, "en")

			want := []domain.NamedCrowdStatus{{CrowdStatus: tt.target, DisplayName: tt.wantName}}
			if !reflect.DeepEqual(view.Crowd, want) || complete != tt.wantComplete {
				t.Errorf("NameCrowd() = %+v, %v, want %+v, %v", view.Crowd, complete, want, tt.wantComplete)
			}
		})
	}
}

func TestNameQueues(t *testing.T) {
	t.Parallel()

	status := domain.GuestStatus{Queues: []domain.QueueStatus{
		{QueueID: "q1", GatePointIDs: []string{"p-east", "p-west"}, WaitTimeMinutes: 12.5, TurnaroundCount: 3, RecommendedGatePointIDs: []string{"p-west"}},
		{QueueID: "q2", GatePointIDs: []string{"p-new", "p-plaza"}, WaitTimeMinutes: 0, TurnaroundCount: 0, RecommendedGatePointIDs: nil},
	}}

	view, complete := testDisplayNames().NameQueues(status, "ja")

	want := []domain.NamedQueueStatus{
		{QueueID: "q1", DisplayName: "東門 / 西門", WaitTimeMinutes: 12.5, TurnaroundCount: 3, Gates: []domain.QueueGate{
			{PointID: "p-east", DisplayName: "東門", Recommended: false},
			{PointID: "p-west", DisplayName: "西門", Recommended: true},
		}},
		{QueueID: "q2", DisplayName: "広場", WaitTimeMinutes: 0, TurnaroundCount: 0, Gates: []domain.QueueGate{
			{PointID: "p-new", DisplayName: "", Recommended: false},
			{PointID: "p-plaza", DisplayName: "広場", Recommended: false},
		}},
	}

	if !reflect.DeepEqual(view.Queues, want) {
		t.Errorf("NameQueues() = %+v, want %+v", view.Queues, want)
	}

	if complete {
		t.Error("NameQueues() complete = true, want false for the unknown gate p-new")
	}
}

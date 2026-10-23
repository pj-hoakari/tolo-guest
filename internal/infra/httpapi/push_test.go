package httpapi_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	observationv1 "github.com/pj-hoakari/tolo-observation/gen/tolo/observation/v1"
	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/infra/httpapi"
)

type fakeReadModel struct {
	err      error
	statuses []domain.GuestStatus
	updates  []domain.GuestMessagesUpdate
}

func (f *fakeReadModel) ApplyGuestStatus(_ context.Context, status domain.GuestStatus) error {
	f.statuses = append(f.statuses, status)

	return f.err
}

func (f *fakeReadModel) ApplyGuestMessages(_ context.Context, update domain.GuestMessagesUpdate) error {
	f.updates = append(f.updates, update)

	return f.err
}

func push(t *testing.T, readModel *fakeReadModel, method, path string, body []byte) int {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Pubsub-Message-Id", "message-1")
	req.Header.Set("X-Goog-Pubsub-Ordering-Key", "event-1")
	req.Header.Set("X-Goog-Pubsub-Publish-Time", "2026-10-23T10:00:00Z")

	rec := httptest.NewRecorder()
	httpapi.NewHandler(httpapi.PushRoutes(readModel)).ServeHTTP(rec, req)

	return rec.Result().StatusCode
}

func marshal(t *testing.T, m proto.Message) []byte {
	t.Helper()

	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return b
}

func isSuccess(status int) bool {
	return status >= 200 && status <= 299
}

func TestPushGuestStatusAppliesTheDecodedStatus(t *testing.T) {
	t.Parallel()

	takenAt := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	readModel := &fakeReadModel{}

	got := push(t, readModel, http.MethodPost, "/push/guest-status", marshal(t, &observationv1.GuestStatus{
		EventId:  "event-1",
		Sequence: 7,
		TakenAt:  timestamppb.New(takenAt),
		Crowd: []*observationv1.CrowdStatus{
			{Target: &observationv1.CrowdStatus_PointId{PointId: "point-1"}, Level: observationv1.CrowdLevel_CROWD_LEVEL_HIGH, Danger: true},
			{Target: &observationv1.CrowdStatus_RouteId{RouteId: "route-1"}, Level: observationv1.CrowdLevel_CROWD_LEVEL_LOW},
		},
		Queues: []*observationv1.QueueStatus{{
			QueueId:                 "queue-1",
			GatePointIds:            []string{"gate-1", "gate-2"},
			WaitTimeMinutes:         12.5,
			TurnaroundCount:         3,
			RecommendedGatePointIds: []string{"gate-2"},
		}},
	}))
	if !isSuccess(got) {
		t.Fatalf("status = %d, want 2xx", got)
	}

	want := []domain.GuestStatus{{
		EventID:  "event-1",
		Sequence: 7,
		TakenAt:  takenAt,
		Crowd: []domain.CrowdStatus{
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: "point-1"}, Level: domain.CrowdLevelHigh, Danger: true},
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetRoute, ID: "route-1"}, Level: domain.CrowdLevelLow, Danger: false},
		},
		Queues: []domain.QueueStatus{{
			QueueID:                 "queue-1",
			GatePointIDs:            []string{"gate-1", "gate-2"},
			WaitTimeMinutes:         12.5,
			TurnaroundCount:         3,
			RecommendedGatePointIDs: []string{"gate-2"},
		}},
	}}
	if !reflect.DeepEqual(readModel.statuses, want) {
		t.Errorf("applied statuses = %+v, want %+v", readModel.statuses, want)
	}
}

func TestPushGuestMessagesAppliesTheDecodedUpdate(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2026, 10, 23, 9, 30, 0, 0, time.UTC)
	readModel := &fakeReadModel{}

	got := push(t, readModel, http.MethodPost, "/push/guest-messages", marshal(t, &operationv1.GuestMessagesUpdate{
		EventId:           "event-1",
		Upserts:           []*operationv1.GuestMessage{{MessageId: "message-a", Body: "東門は混雑しています", SentAt: timestamppb.New(sentAt)}},
		RevokedMessageIds: []string{"message-b"},
	}))
	if !isSuccess(got) {
		t.Fatalf("status = %d, want 2xx", got)
	}

	want := []domain.GuestMessagesUpdate{{
		EventID:           "event-1",
		Upserts:           []domain.GuestMessage{{MessageID: "message-a", Body: "東門は混雑しています", SentAt: sentAt}},
		RevokedMessageIDs: []string{"message-b"},
	}}
	if !reflect.DeepEqual(readModel.updates, want) {
		t.Errorf("applied updates = %+v, want %+v", readModel.updates, want)
	}
}

func TestPushThatCannotBeDecodedIsAcknowledgedWithoutApplying(t *testing.T) {
	t.Parallel()

	payloads := map[string][]byte{
		"truncated bytes":  {0xff},
		"missing event_id": marshal(t, &observationv1.GuestStatus{Sequence: 1}),
		"oversized body":   make([]byte, 10<<20+1),
	}

	for _, path := range []string{"/push/guest-status", "/push/guest-messages"} {
		for name, payload := range payloads {
			t.Run(path+" "+name, func(t *testing.T) {
				t.Parallel()

				readModel := &fakeReadModel{}

				if got := push(t, readModel, http.MethodPost, path, payload); !isSuccess(got) {
					t.Errorf("status = %d, want 2xx so the broker does not redeliver", got)
				}

				if len(readModel.statuses)+len(readModel.updates) != 0 {
					t.Errorf("read model applied %d statuses and %d updates, want none", len(readModel.statuses), len(readModel.updates))
				}
			})
		}
	}
}

func TestPushThatFailsToApplyAsksForRedelivery(t *testing.T) {
	t.Parallel()

	bodies := map[string][]byte{
		"/push/guest-status":   marshal(t, &observationv1.GuestStatus{EventId: "event-1", Sequence: 1}),
		"/push/guest-messages": marshal(t, &operationv1.GuestMessagesUpdate{EventId: "event-1", RevokedMessageIds: []string{"message-b"}}),
	}

	for path, body := range bodies {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			readModel := &fakeReadModel{err: errors.New("database is down")}

			if got := push(t, readModel, http.MethodPost, path, body); got < 500 || got > 599 {
				t.Errorf("status = %d, want 5xx so the broker redelivers", got)
			}
		})
	}
}

func TestPushRejectsMethodsOtherThanPost(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/push/guest-status", "/push/guest-messages"} {
		if got := push(t, &fakeReadModel{}, http.MethodGet, path, nil); got != http.StatusMethodNotAllowed {
			t.Errorf("GET %s status = %d, want %d", path, got, http.StatusMethodNotAllowed)
		}
	}
}

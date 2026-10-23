package connect

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	guestv1 "github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1"
	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

type memoryReadModel struct {
	mu       sync.Mutex
	statuses map[string]domain.GuestStatus
	names    map[string]domain.DisplayNames
	messages map[string][]domain.GuestMessage
	qr       map[string]domain.QrLocations
}

func newMemoryReadModel() *memoryReadModel {
	return &memoryReadModel{
		statuses: map[string]domain.GuestStatus{},
		names:    map[string]domain.DisplayNames{},
		messages: map[string][]domain.GuestMessage{},
		qr:       map[string]domain.QrLocations{},
	}
}

func newGuestPageUseCases(readModel *memoryReadModel) *application.GuestReadModelService {
	return application.NewGuestReadModelService(readModel, &memoryGuestSettings{byEvent: map[string]domain.GuestSettings{}}, nil, nil, nil, nil)
}

func (m *memoryReadModel) ApplyStatus(_ context.Context, status domain.GuestStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.statuses[status.EventID] = status

	return nil
}

func (m *memoryReadModel) ApplyMessages(context.Context, domain.GuestMessagesUpdate) error { return nil }

func (m *memoryReadModel) ReplaceMessages(_ context.Context, eventID string, messages []domain.GuestMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages[eventID] = messages

	return nil
}

func (m *memoryReadModel) ListEventIDs(context.Context) ([]string, error) { return nil, nil }

func (m *memoryReadModel) FindStatus(_ context.Context, eventID string) (domain.GuestStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, ok := m.statuses[eventID]
	if !ok {
		return domain.GuestStatus{}, repository.ErrGuestStatusNotFound
	}

	return status, nil
}

func (m *memoryReadModel) ListMessages(_ context.Context, eventID string) ([]domain.GuestMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.messages[eventID], nil
}

func (m *memoryReadModel) SaveDisplayNames(_ context.Context, names domain.DisplayNames) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.names[names.EventID] = names

	return nil
}

func (m *memoryReadModel) FindDisplayNames(_ context.Context, eventID string) (domain.DisplayNames, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	names, ok := m.names[eventID]
	if !ok {
		return domain.DisplayNames{}, repository.ErrDisplayNamesNotFound
	}

	return names, nil
}

func (m *memoryReadModel) SaveQrLocations(_ context.Context, locations domain.QrLocations) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.qr[locations.EventID] = locations

	return nil
}

func (m *memoryReadModel) FindQrLocations(_ context.Context, eventID string) (domain.QrLocations, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	locations, ok := m.qr[eventID]
	if !ok {
		return domain.QrLocations{}, repository.ErrQrLocationsNotFound
	}

	return locations, nil
}

func newGuestPageClient(t *testing.T) guestv1connect.GuestPageServiceClient {
	t.Helper()

	ctx := context.Background()
	takenAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	readModel := newMemoryReadModel()

	_ = readModel.ApplyStatus(ctx, domain.GuestStatus{
		EventID:  testEventPublicID,
		Sequence: 1,
		TakenAt:  takenAt,
		Crowd: []domain.CrowdStatus{
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: "p-east"}, Level: domain.CrowdLevelHigh, Danger: true},
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetRoute, ID: "r1"}, Level: domain.CrowdLevelLow},
		},
		Queues: []domain.QueueStatus{{
			QueueID:                 "q1",
			GatePointIDs:            []string{"p-east", "p-west"},
			WaitTimeMinutes:         12.5,
			TurnaroundCount:         3,
			RecommendedGatePointIDs: []string{"p-west"},
		}},
	})
	_ = readModel.SaveDisplayNames(ctx, domain.DisplayNames{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Points: map[string]domain.LocalizedText{
			"p-east": {"ja": "東門", "en": "East Gate"},
			"p-west": {"ja": "西門", "en": "West Gate"},
		},
		RouteEndpoints: map[string]domain.RouteEndpoints{"r1": {FromPointID: "p-east", ToPointID: "p-west"}},
	})
	_ = readModel.ReplaceMessages(ctx, testEventPublicID, []domain.GuestMessage{
		{MessageID: "m1", Body: "Gate A is open", SentAt: takenAt},
	})

	routes, err := RoutesWithVerifier(newTestVerifier(t, internaljwt.JWKS{}), newGuestSettingsUseCases(), newGuestPageUseCases(readModel))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return guestv1connect.NewGuestPageServiceClient(newClient(newTestServer(t, mux)))
}

func TestGuestPageComponents(t *testing.T) {
	t.Parallel()

	client := newGuestPageClient(t)
	ctx := context.Background()
	takenAt := timestamppb.New(time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC))

	t.Run("GetCrowd names points and routes in the requested language without a token", func(t *testing.T) {
		t.Parallel()

		got, err := client.GetCrowd(ctx, &guestv1.GetCrowdRequest{EventId: testEventPublicID, Language: "en"})
		if err != nil {
			t.Fatalf("GetCrowd() error = %v", err)
		}

		want := &guestv1.GetCrowdResponse{TakenAt: takenAt, Crowd: []*guestv1.Crowd{
			{Target: &guestv1.Crowd_PointId{PointId: "p-east"}, DisplayName: "East Gate", Level: guestv1.CrowdLevel_CROWD_LEVEL_HIGH, Danger: true},
			{Target: &guestv1.Crowd_RouteId{RouteId: "r1"}, DisplayName: "East Gate → West Gate", Level: guestv1.CrowdLevel_CROWD_LEVEL_LOW},
		}}
		if !proto.Equal(got, want) {
			t.Errorf("GetCrowd() = %v, want %v", got, want)
		}
	})

	t.Run("GetQueues labels a queue from its gates and marks the recommended gate", func(t *testing.T) {
		t.Parallel()

		got, err := client.GetQueues(ctx, &guestv1.GetQueuesRequest{EventId: testEventPublicID, Language: "ja"})
		if err != nil {
			t.Fatalf("GetQueues() error = %v", err)
		}

		want := &guestv1.GetQueuesResponse{TakenAt: takenAt, Queues: []*guestv1.Queue{{
			QueueId:         "q1",
			DisplayName:     "東門 / 西門",
			WaitTimeMinutes: 12.5,
			TurnaroundCount: 3,
			Gates: []*guestv1.QueueGate{
				{PointId: "p-east", DisplayName: "東門"},
				{PointId: "p-west", DisplayName: "西門", Recommended: true},
			},
		}}}
		if !proto.Equal(got, want) {
			t.Errorf("GetQueues() = %v, want %v", got, want)
		}
	})

	t.Run("ListGuestMessages returns the stored messages without a token", func(t *testing.T) {
		t.Parallel()

		got, err := client.ListGuestMessages(ctx, &guestv1.ListGuestMessagesRequest{EventId: testEventPublicID})
		if err != nil {
			t.Fatalf("ListGuestMessages() error = %v", err)
		}

		want := &guestv1.ListGuestMessagesResponse{Messages: []*guestv1.GuestMessage{
			{MessageId: "m1", Body: "Gate A is open", SentAt: takenAt},
		}}
		if !proto.Equal(got, want) {
			t.Errorf("ListGuestMessages() = %v, want %v", got, want)
		}
	})

	t.Run("an event without a status is served empty without taken_at", func(t *testing.T) {
		t.Parallel()

		crowd, err := client.GetCrowd(ctx, &guestv1.GetCrowdRequest{EventId: otherEventPublicID})
		if err != nil || crowd.GetTakenAt() != nil || len(crowd.GetCrowd()) != 0 {
			t.Errorf("GetCrowd() = %v, %v, want an empty response", crowd, err)
		}

		queues, err := client.GetQueues(ctx, &guestv1.GetQueuesRequest{EventId: otherEventPublicID})
		if err != nil || queues.GetTakenAt() != nil || len(queues.GetQueues()) != 0 {
			t.Errorf("GetQueues() = %v, %v, want an empty response", queues, err)
		}

		messages, err := client.ListGuestMessages(ctx, &guestv1.ListGuestMessagesRequest{EventId: otherEventPublicID})
		if err != nil || len(messages.GetMessages()) != 0 {
			t.Errorf("ListGuestMessages() = %v, %v, want an empty response", messages, err)
		}
	})

	t.Run("a missing event ID is an invalid argument", func(t *testing.T) {
		t.Parallel()

		_, err := client.GetCrowd(ctx, &guestv1.GetCrowdRequest{})
		assertConnectError(t, err, connectrpc.CodeInvalidArgument, "event ID is required")

		_, err = client.GetQueues(ctx, &guestv1.GetQueuesRequest{})
		assertConnectError(t, err, connectrpc.CodeInvalidArgument, "event ID is required")

		_, err = client.ListGuestMessages(ctx, &guestv1.ListGuestMessagesRequest{})
		assertConnectError(t, err, connectrpc.CodeInvalidArgument, "event ID is required")
	})
}

package connect

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"google.golang.org/protobuf/types/known/timestamppb"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	observationv1 "github.com/pj-hoakari/tolo-observation/gen/tolo/observation/v1"
	"github.com/pj-hoakari/tolo-observation/gen/tolo/observation/v1/observationv1connect"
	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"

	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

type upstreamCall struct {
	callerService string
	authorization bool
}

func recordCall(ctx context.Context) upstreamCall {
	info, _ := connectrpc.CallInfoForServerContext(ctx)

	return upstreamCall{
		callerService: info.RequestHeader().Get("Tolo-Caller-Service"),
		authorization: info.RequestHeader().Has("Authorization"),
	}
}

type fakeStatusQuery struct {
	observationv1connect.UnimplementedStatusQueryServiceHandler

	calls  chan upstreamCall
	status *observationv1.GuestStatus
}

func (f *fakeStatusQuery) GetGuestSnapshot(
	ctx context.Context,
	req *observationv1.GetGuestSnapshotRequest,
) (*observationv1.GetGuestSnapshotResponse, error) {
	f.calls <- recordCall(ctx)

	if f.status == nil || req.GetEventId() != f.status.GetEventId() {
		return nil, connectrpc.NewError(connectrpc.CodeNotFound, "guest status not found")
	}

	return &observationv1.GetGuestSnapshotResponse{Status: f.status}, nil
}

type fakeMeasurementIngest struct {
	observationv1connect.UnimplementedMeasurementIngestServiceHandler

	calls    chan upstreamCall
	requests chan *observationv1.ReportMeasurementsRequest
	refuse   connectrpc.Code
}

func (f *fakeMeasurementIngest) ReportMeasurements(
	ctx context.Context,
	req *observationv1.ReportMeasurementsRequest,
) (*observationv1.ReportMeasurementsResponse, error) {
	f.calls <- recordCall(ctx)

	f.requests <- req

	if f.refuse != 0 {
		return nil, connectrpc.NewError(f.refuse, "refused")
	}

	return &observationv1.ReportMeasurementsResponse{AcceptedCount: int32(len(req.GetMeasurements()))}, nil
}

type fakeStaffCommunication struct {
	operationv1connect.UnimplementedStaffCommunicationServiceHandler

	calls    chan upstreamCall
	requests chan *operationv1.ListStaffMessagesRequest
	messages []*operationv1.StaffMessage
}

func (f *fakeStaffCommunication) ListStaffMessages(
	ctx context.Context,
	req *operationv1.ListStaffMessagesRequest,
) (*operationv1.ListStaffMessagesResponse, error) {
	f.calls <- recordCall(ctx)

	f.requests <- req

	return &operationv1.ListStaffMessagesResponse{Messages: f.messages}, nil
}

type fakeGraphSupply struct {
	graphv1connect.UnimplementedGraphSupplyServiceHandler

	calls       chan upstreamCall
	names       *graphv1.GetDisplayNamesResponse
	qrRequests  chan *graphv1.GetQrLocationsRequest
	qrLocations []*graphv1.QrLocation
}

func (f *fakeGraphSupply) GetDisplayNames(
	ctx context.Context,
	req *graphv1.GetDisplayNamesRequest,
) (*graphv1.GetDisplayNamesResponse, error) {
	f.calls <- recordCall(ctx)

	if req.GetEventId() != testEventPublicID {
		return nil, connectrpc.NewError(connectrpc.CodeNotFound, "revision not found")
	}

	return f.names, nil
}

func (f *fakeGraphSupply) GetQrLocations(
	ctx context.Context,
	req *graphv1.GetQrLocationsRequest,
) (*graphv1.GetQrLocationsResponse, error) {
	f.calls <- recordCall(ctx)

	f.qrRequests <- req

	if req.GetRevisionId() != "rev-1" {
		return nil, connectrpc.NewError(connectrpc.CodeNotFound, "revision not found")
	}

	return &graphv1.GetQrLocationsResponse{QrLocations: f.qrLocations, RevisionId: req.GetRevisionId()}, nil
}

func newGatewayServer(t *testing.T, register func(*connectrpc.Server)) string {
	t.Helper()

	server := connectrpc.NewServer()
	register(server)

	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)

	return newTestServer(t, mux).URL
}

func assertNewMachineCaller(t *testing.T, got upstreamCall) {
	t.Helper()

	if got.callerService != "tolo-guest" {
		t.Errorf("Tolo-Caller-Service = %q, want tolo-guest", got.callerService)
	}

	if got.authorization {
		t.Error("the request carries an Authorization header, want none")
	}
}

func TestObservationClient(t *testing.T) {
	t.Parallel()

	takenAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	fake := &fakeStatusQuery{
		calls: make(chan upstreamCall, 2),
		status: &observationv1.GuestStatus{
			EventId:  testEventPublicID,
			Sequence: 42,
			TakenAt:  timestamppb.New(takenAt),
			Crowd: []*observationv1.CrowdStatus{
				{Target: &observationv1.CrowdStatus_PointId{PointId: "p1"}, Level: observationv1.CrowdLevel_CROWD_LEVEL_MID},
				{Target: &observationv1.CrowdStatus_RouteId{RouteId: "r1"}, Level: observationv1.CrowdLevel_CROWD_LEVEL_HIGH, Danger: true},
			},
			Queues: []*observationv1.QueueStatus{{
				QueueId:                 "q1",
				GatePointIds:            []string{"g1", "g2"},
				WaitTimeMinutes:         7.5,
				TurnaroundCount:         2,
				RecommendedGatePointIds: []string{"g2"},
			}},
		},
	}
	url := newGatewayServer(t, func(server *connectrpc.Server) {
		observationv1connect.RegisterStatusQueryServiceHandler(server, fake)
	})
	client := NewObservationClient(http.DefaultClient, url)

	got, found, err := client.GetGuestSnapshot(authorized("Bearer leaked"), testEventPublicID)
	if err != nil || !found {
		t.Fatalf("GetGuestSnapshot() = _, %v, %v, want found", found, err)
	}

	want := domain.GuestStatus{
		EventID:  testEventPublicID,
		Sequence: 42,
		TakenAt:  takenAt,
		Crowd: []domain.CrowdStatus{
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: "p1"}, Level: domain.CrowdLevelMid, Danger: false},
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetRoute, ID: "r1"}, Level: domain.CrowdLevelHigh, Danger: true},
		},
		Queues: []domain.QueueStatus{{
			QueueID:                 "q1",
			GatePointIDs:            []string{"g1", "g2"},
			WaitTimeMinutes:         7.5,
			TurnaroundCount:         2,
			RecommendedGatePointIDs: []string{"g2"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetGuestSnapshot() = %+v, want %+v", got, want)
	}

	assertNewMachineCaller(t, <-fake.calls)

	if _, found, err := client.GetGuestSnapshot(context.Background(), otherEventPublicID); err != nil || found {
		t.Errorf("GetGuestSnapshot() for an event without a status = _, %v, %v, want not found without error", found, err)
	}

	assertNewMachineCaller(t, <-fake.calls)
}

func TestObservationClientReportsQrCounts(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	counts := []domain.QrCount{
		{QrLocationID: "qr-east", WindowStart: start, WindowEnd: start.Add(time.Minute), Count: 3, AccessIDs: []int64{1, 2, 3}},
		{QrLocationID: "qr-hall", WindowStart: start, WindowEnd: start.Add(time.Minute), Count: 1, AccessIDs: []int64{4}},
	}

	fake := &fakeMeasurementIngest{
		calls:    make(chan upstreamCall, 1),
		requests: make(chan *observationv1.ReportMeasurementsRequest, 1),
	}
	url := newGatewayServer(t, func(server *connectrpc.Server) {
		observationv1connect.RegisterMeasurementIngestServiceHandler(server, fake)
	})

	err := NewObservationClient(http.DefaultClient, url).ReportQrCounts(authorized("Bearer leaked"), testTenantPublicID, testEventPublicID, counts)
	if err != nil {
		t.Fatalf("ReportQrCounts() error = %v", err)
	}

	assertNewMachineCaller(t, <-fake.calls)

	req := <-fake.requests
	if req.GetEventId() != testEventPublicID || req.GetTenantId() != testTenantPublicID || req.GetEdgeDeviceId() != "" {
		t.Errorf("ReportMeasurements request = %v, want the event and its tenant without an edge device", req)
	}

	measurements := req.GetMeasurements()
	if len(measurements) != len(counts) {
		t.Fatalf("measurements = %v, want one per count", measurements)
	}

	for i, m := range measurements {
		want := counts[i]
		if m.GetSource() != observationv1.MeasurementSource_MEASUREMENT_SOURCE_QR || m.GetQrLocationId() != want.QrLocationID ||
			m.GetObservationPointId() != "" || m.GetCountIn() != want.Count || m.GetCountOut() != 0 || m.MeanDetectedPeople != nil ||
			!m.GetWindowStart().AsTime().Equal(want.WindowStart) || !m.GetWindowEnd().AsTime().Equal(want.WindowEnd) {
			t.Errorf("measurement[%d] = %v, want a QR count of %+v with count_out 0", i, m, want)
		}
	}
}

func TestObservationClientMarksRefusedQrCounts(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	counts := []domain.QrCount{{QrLocationID: "qr-east", WindowStart: start, WindowEnd: start.Add(time.Minute), Count: 1, AccessIDs: []int64{1}}}

	for _, tc := range []struct {
		code    connectrpc.Code
		refused bool
	}{
		{connectrpc.CodeInvalidArgument, true},
		{connectrpc.CodePermissionDenied, true},
		{connectrpc.CodeFailedPrecondition, true},
		{connectrpc.CodeUnavailable, false},
		{connectrpc.CodeUnauthenticated, false},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			t.Parallel()

			fake := &fakeMeasurementIngest{
				calls:    make(chan upstreamCall, 1),
				requests: make(chan *observationv1.ReportMeasurementsRequest, 1),
				refuse:   tc.code,
			}
			url := newGatewayServer(t, func(server *connectrpc.Server) {
				observationv1connect.RegisterMeasurementIngestServiceHandler(server, fake)
			})

			err := NewObservationClient(http.DefaultClient, url).ReportQrCounts(context.Background(), testTenantPublicID, testEventPublicID, counts)
			if err == nil {
				t.Fatal("ReportQrCounts() error = nil, want the refusal")
			}

			if got := errors.Is(err, application.ErrQrCountsRefused); got != tc.refused {
				t.Errorf("errors.Is(%v, ErrQrCountsRefused) = %v, want %v", err, got, tc.refused)
			}
		})
	}
}

func TestOperationClient(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2026, 10, 23, 9, 30, 0, 0, time.UTC)
	fake := &fakeStaffCommunication{
		calls:    make(chan upstreamCall, 1),
		requests: make(chan *operationv1.ListStaffMessagesRequest, 1),
		messages: []*operationv1.StaffMessage{{
			MessageId: "m1",
			EventId:   testEventPublicID,
			Audience:  operationv1.Audience_AUDIENCE_BOTH,
			Body:      "Gate A is open",
			SentAt:    timestamppb.New(sentAt),
		}},
	}
	url := newGatewayServer(t, func(server *connectrpc.Server) {
		operationv1connect.RegisterStaffCommunicationServiceHandler(server, fake)
	})

	got, err := NewOperationClient(http.DefaultClient, url).ListGuestMessages(context.Background(), testTenantPublicID, testEventPublicID)
	if err != nil {
		t.Fatalf("ListGuestMessages() error = %v", err)
	}

	want := []domain.GuestMessage{{MessageID: "m1", Body: "Gate A is open", SentAt: sentAt}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListGuestMessages() = %+v, want %+v", got, want)
	}

	assertNewMachineCaller(t, <-fake.calls)

	req := <-fake.requests
	if req.GetEventId() != testEventPublicID || req.GetTenantId() != testTenantPublicID ||
		req.GetAudience() != operationv1.Audience_AUDIENCE_GUEST {
		t.Errorf("ListStaffMessages request = %v, want the guest audience of the event and its tenant", req)
	}
}

func TestGraphAuthoringClient(t *testing.T) {
	t.Parallel()

	fake := &fakeGraphSupply{
		calls:      make(chan upstreamCall, 2),
		qrRequests: make(chan *graphv1.GetQrLocationsRequest, 2),
		qrLocations: []*graphv1.QrLocation{
			{QrLocationId: "qr-east", Name: "東門", Kind: "entrance"},
			{QrLocationId: "qr-hall", Name: "ホール前", Kind: "queue"},
		},
		names: &graphv1.GetDisplayNamesResponse{
			RevisionId: "rev-1",
			DisplayNames: map[string]*graphv1.LocalizedText{
				"p1": {Texts: map[string]string{"ja": "東門", "en": "East Gate"}},
				"p2": {Texts: map[string]string{"ja": "西門"}},
			},
			RouteDisplayNames: map[string]string{"r1": "メイン通り"},
			RouteEndpoints: map[string]*graphv1.RouteEndpoints{
				"r1": {FromPointId: "p1", ToPointId: "p2"},
				"r2": {FromPointId: "p2", ToPointId: "p1"},
			},
		},
	}
	url := newGatewayServer(t, func(server *connectrpc.Server) {
		graphv1connect.RegisterGraphSupplyServiceHandler(server, fake)
	})
	client := NewGraphAuthoringClient(http.DefaultClient, url)

	got, found, err := client.GetDisplayNames(authorized("Bearer leaked"), testEventPublicID)
	if err != nil || !found {
		t.Fatalf("GetDisplayNames() = _, %v, %v, want found", found, err)
	}

	want := domain.DisplayNames{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Points: map[string]domain.LocalizedText{
			"p1": {"ja": "東門", "en": "East Gate"},
			"p2": {"ja": "西門"},
		},
		Routes: map[string]string{"r1": "メイン通り"},
		RouteEndpoints: map[string]domain.RouteEndpoints{
			"r1": {FromPointID: "p1", ToPointID: "p2"},
			"r2": {FromPointID: "p2", ToPointID: "p1"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetDisplayNames() = %+v, want %+v", got, want)
	}

	assertNewMachineCaller(t, <-fake.calls)

	if _, found, err := client.GetDisplayNames(context.Background(), otherEventPublicID); err != nil || found {
		t.Errorf("GetDisplayNames() for an event without a published revision = _, %v, %v, want not found without error", found, err)
	}

	assertNewMachineCaller(t, <-fake.calls)

	locations, found, err := client.GetQrLocations(authorized("Bearer leaked"), testTenantPublicID, testEventPublicID, "rev-1")
	if err != nil || !found {
		t.Fatalf("GetQrLocations() = _, %v, %v, want found", found, err)
	}

	wantLocations := domain.QrLocations{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Locations: []domain.QrLocation{
			{QrLocationID: "qr-east", Name: "東門", Kind: "entrance"},
			{QrLocationID: "qr-hall", Name: "ホール前", Kind: "queue"},
		},
	}
	if !reflect.DeepEqual(locations, wantLocations) {
		t.Errorf("GetQrLocations() = %+v, want %+v", locations, wantLocations)
	}

	assertNewMachineCaller(t, <-fake.calls)

	req := <-fake.qrRequests
	if req.GetEventId() != testEventPublicID || req.GetRevisionId() != "rev-1" || req.GetTenantId() != testTenantPublicID {
		t.Errorf("GetQrLocations request = %v, want the event, the revision and the tenant", req)
	}

	if _, found, err := client.GetQrLocations(context.Background(), testTenantPublicID, testEventPublicID, "rev-gone"); err != nil || found {
		t.Errorf("GetQrLocations() for an unpublished revision = _, %v, %v, want not found without error", found, err)
	}

	assertNewMachineCaller(t, <-fake.calls)
	<-fake.qrRequests
}

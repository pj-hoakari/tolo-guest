package connect

import (
	"context"
	"fmt"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"google.golang.org/protobuf/proto"
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

const (
	callerServiceHeader = "Tolo-Caller-Service"
	callerService       = "tolo-guest"
)

var crowdLevels = map[observationv1.CrowdLevel]domain.CrowdLevel{
	observationv1.CrowdLevel_CROWD_LEVEL_UNSPECIFIED: domain.CrowdLevelUnspecified,
	observationv1.CrowdLevel_CROWD_LEVEL_LOW:         domain.CrowdLevelLow,
	observationv1.CrowdLevel_CROWD_LEVEL_MID:         domain.CrowdLevelMid,
	observationv1.CrowdLevel_CROWD_LEVEL_HIGH:        domain.CrowdLevelHigh,
}

func newGatewayClient(httpClient connecthttp.HTTPClient, gatewayURL string) *connectrpc.Client {
	return connectrpc.NewClient(connecthttp.NewTransport(httpClient, gatewayURL), declareCallerService())
}

func declareCallerService() connectrpc.ClientInterceptor {
	return func(next connectrpc.ClientFunc) connectrpc.ClientFunc {
		return func(ctx context.Context, spec connectrpc.Spec) (connectrpc.ClientStream, error) {
			if info, ok := connectrpc.CallInfoForClientContext(ctx); ok {
				info.RequestHeader().Delete("Authorization")
				info.RequestHeader().Set(callerServiceHeader, callerService)
			}

			return next(ctx, spec)
		}
	}
}

type ObservationClient struct {
	client       observationv1connect.StatusQueryServiceClient
	measurements observationv1connect.MeasurementIngestServiceClient
}

func NewObservationClient(httpClient connecthttp.HTTPClient, gatewayURL string) *ObservationClient {
	gateway := newGatewayClient(httpClient, gatewayURL)

	return &ObservationClient{
		client:       observationv1connect.NewStatusQueryServiceClient(gateway),
		measurements: observationv1connect.NewMeasurementIngestServiceClient(gateway),
	}
}

func (c *ObservationClient) ReportQrCounts(ctx context.Context, tenantPublicID, eventID string, counts []domain.QrCount) error {
	measurements := make([]*observationv1.Measurement, 0, len(counts))
	for _, count := range counts {
		measurements = append(measurements, &observationv1.Measurement{
			ObservationPointId: "",
			WindowStart:        timestamppb.New(count.WindowStart),
			WindowEnd:          timestamppb.New(count.WindowEnd),
			CountIn:            count.Count,
			CountOut:           0,
			Source:             observationv1.MeasurementSource_MEASUREMENT_SOURCE_QR,
			QrLocationId:       count.QrLocationID,
			MeanDetectedPeople: nil,
		})
	}

	_, err := c.measurements.ReportMeasurements(ctx, &observationv1.ReportMeasurementsRequest{
		EventId:      eventID,
		EdgeDeviceId: "",
		Measurements: measurements,
		TenantId:     tenantPublicID,
	})

	if code := connectrpc.CodeOf(err); code == connectrpc.CodeInvalidArgument ||
		code == connectrpc.CodePermissionDenied || code == connectrpc.CodeFailedPrecondition {
		return fmt.Errorf("call ReportMeasurements: %w: %w", application.ErrQrCountsRefused, err)
	}

	if err != nil {
		return fmt.Errorf("call ReportMeasurements: %w", err)
	}

	return nil
}

func (c *ObservationClient) GetGuestSnapshot(ctx context.Context, eventID string) (domain.GuestStatus, bool, error) {
	res, err := c.client.GetGuestSnapshot(ctx, &observationv1.GetGuestSnapshotRequest{EventId: eventID})
	if connectrpc.CodeOf(err) == connectrpc.CodeNotFound {
		var none domain.GuestStatus

		return none, false, nil
	}

	if err != nil {
		return domain.GuestStatus{}, false, fmt.Errorf("call GetGuestSnapshot: %w", err)
	}

	return GuestStatus(res.GetStatus()), true, nil
}

func GuestStatus(status *observationv1.GuestStatus) domain.GuestStatus {
	crowd := make([]domain.CrowdStatus, 0, len(status.GetCrowd()))
	for _, c := range status.GetCrowd() {
		target := domain.CrowdTarget{Kind: domain.CrowdTargetUnspecified, ID: ""}

		switch t := c.GetTarget().(type) {
		case *observationv1.CrowdStatus_PointId:
			target = domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: t.PointId}
		case *observationv1.CrowdStatus_RouteId:
			target = domain.CrowdTarget{Kind: domain.CrowdTargetRoute, ID: t.RouteId}
		}

		crowd = append(crowd, domain.CrowdStatus{Target: target, Level: crowdLevels[c.GetLevel()], Danger: c.GetDanger()})
	}

	queues := make([]domain.QueueStatus, 0, len(status.GetQueues()))
	for _, q := range status.GetQueues() {
		queues = append(queues, domain.QueueStatus{
			QueueID:                 q.GetQueueId(),
			GatePointIDs:            q.GetGatePointIds(),
			WaitTimeMinutes:         q.GetWaitTimeMinutes(),
			TurnaroundCount:         q.GetTurnaroundCount(),
			RecommendedGatePointIDs: q.GetRecommendedGatePointIds(),
		})
	}

	return domain.GuestStatus{
		EventID:  status.GetEventId(),
		Sequence: status.GetSequence(),
		TakenAt:  status.GetTakenAt().AsTime(),
		Crowd:    crowd,
		Queues:   queues,
	}
}

func GuestMessagesUpdate(update *operationv1.GuestMessagesUpdate) domain.GuestMessagesUpdate {
	upserts := make([]domain.GuestMessage, 0, len(update.GetUpserts()))
	for _, message := range update.GetUpserts() {
		upserts = append(upserts, domain.GuestMessage{
			MessageID: message.GetMessageId(),
			Body:      message.GetBody(),
			SentAt:    message.GetSentAt().AsTime(),
		})
	}

	return domain.GuestMessagesUpdate{
		EventID:           update.GetEventId(),
		Upserts:           upserts,
		RevokedMessageIDs: update.GetRevokedMessageIds(),
	}
}

type OperationClient struct {
	client operationv1connect.StaffCommunicationServiceClient
}

func NewOperationClient(httpClient connecthttp.HTTPClient, gatewayURL string) *OperationClient {
	return &OperationClient{client: operationv1connect.NewStaffCommunicationServiceClient(newGatewayClient(httpClient, gatewayURL))}
}

func (c *OperationClient) ListGuestMessages(ctx context.Context, tenantPublicID, eventID string) ([]domain.GuestMessage, error) {
	res, err := c.client.ListStaffMessages(ctx, &operationv1.ListStaffMessagesRequest{
		EventId:  eventID,
		Audience: operationv1.Audience_AUDIENCE_GUEST,
		TenantId: tenantPublicID,
	})
	if err != nil {
		return nil, fmt.Errorf("call ListStaffMessages: %w", err)
	}

	messages := make([]domain.GuestMessage, 0, len(res.GetMessages()))
	for _, message := range res.GetMessages() {
		messages = append(messages, domain.GuestMessage{
			MessageID: message.GetMessageId(),
			Body:      message.GetBody(),
			SentAt:    message.GetSentAt().AsTime(),
		})
	}

	return messages, nil
}

type GraphAuthoringClient struct {
	client graphv1connect.GraphSupplyServiceClient
}

func NewGraphAuthoringClient(httpClient connecthttp.HTTPClient, gatewayURL string) *GraphAuthoringClient {
	return &GraphAuthoringClient{client: graphv1connect.NewGraphSupplyServiceClient(newGatewayClient(httpClient, gatewayURL))}
}

func (c *GraphAuthoringClient) GetDisplayNames(ctx context.Context, eventID string) (domain.DisplayNames, bool, error) {
	res, err := c.client.GetDisplayNames(ctx, &graphv1.GetDisplayNamesRequest{EventId: eventID})
	if connectrpc.CodeOf(err) == connectrpc.CodeNotFound {
		var none domain.DisplayNames

		return none, false, nil
	}

	if err != nil {
		return domain.DisplayNames{}, false, fmt.Errorf("call GetDisplayNames: %w", err)
	}

	points := make(map[string]domain.LocalizedText, len(res.GetDisplayNames()))
	for pointID, text := range res.GetDisplayNames() {
		points[pointID] = text.GetTexts()
	}

	endpoints := make(map[string]domain.RouteEndpoints, len(res.GetRouteEndpoints()))
	for routeID, e := range res.GetRouteEndpoints() {
		endpoints[routeID] = domain.RouteEndpoints{FromPointID: e.GetFromPointId(), ToPointID: e.GetToPointId()}
	}

	return domain.DisplayNames{
		EventID:        eventID,
		RevisionID:     res.GetRevisionId(),
		Points:         points,
		Routes:         res.GetRouteDisplayNames(),
		RouteEndpoints: endpoints,
	}, true, nil
}

func (c *GraphAuthoringClient) GetQrLocations(
	ctx context.Context,
	tenantPublicID, eventID, revisionID string,
) (domain.QrLocations, bool, error) {
	res, err := c.client.GetQrLocations(ctx, &graphv1.GetQrLocationsRequest{
		EventId:    eventID,
		RevisionId: revisionID,
		TenantId:   tenantPublicID,
	})
	if connectrpc.CodeOf(err) == connectrpc.CodeNotFound {
		var none domain.QrLocations

		return none, false, nil
	}

	if err != nil {
		return domain.QrLocations{}, false, fmt.Errorf("call GetQrLocations: %w", err)
	}

	locations := make([]domain.QrLocation, 0, len(res.GetQrLocations()))
	for _, location := range res.GetQrLocations() {
		locations = append(locations, domain.QrLocation{
			QrLocationID: location.GetQrLocationId(),
			Name:         location.GetName(),
			Kind:         location.GetKind(),
			PointID:      location.GetAnchor().GetPointId(),
		})
	}

	return domain.QrLocations{EventID: eventID, RevisionID: revisionID, Locations: locations}, true, nil
}

func (c *GraphAuthoringClient) GetGuideMap(
	ctx context.Context,
	tenantPublicID, eventID, revisionID string,
) (*domain.GuideMap, bool, error) {
	res, err := c.client.GetGuideMapLayout(ctx, &graphv1.GetGuideMapLayoutRequest{
		EventId:    eventID,
		RevisionId: revisionID,
		TenantId:   tenantPublicID,
	})
	if connectrpc.CodeOf(err) == connectrpc.CodeNotFound {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("call GetGuideMapLayout: %w", err)
	}

	if res.GetGuideMapLayout() == nil {
		return nil, true, nil
	}

	encoded, err := proto.Marshal(guideMapLayoutFromGraph(res.GetGuideMapLayout()))
	if err != nil {
		return nil, false, fmt.Errorf("encode guide map layout: %w", err)
	}

	return &domain.GuideMap{
		EventID:       eventID,
		RevisionID:    revisionID,
		EncodedLayout: encoded,
		Points:        localizedTexts(res.GetPointLabels()),
		Groups:        localizedTexts(res.GetGroupLabels()),
	}, true, nil
}

func localizedTexts(texts map[string]*graphv1.LocalizedText) map[string]domain.LocalizedText {
	converted := make(map[string]domain.LocalizedText, len(texts))
	for id, text := range texts {
		converted[id] = text.GetTexts()
	}

	return converted
}

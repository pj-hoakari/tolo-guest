package connect

import (
	"context"
	"net/http"
	"testing"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"

	guestv1 "github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1"
	"github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1/guestv1connect"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

type fakeGuideMapSupply struct {
	graphv1connect.UnimplementedGraphSupplyServiceHandler

	responses map[string]*graphv1.GetGuideMapLayoutResponse
}

func (f *fakeGuideMapSupply) GetGuideMapLayout(
	_ context.Context,
	req *graphv1.GetGuideMapLayoutRequest,
) (*graphv1.GetGuideMapLayoutResponse, error) {
	res, ok := f.responses[req.GetRevisionId()]
	if !ok || req.GetTenantId() != testTenantPublicID {
		return nil, connectrpc.NewError(connectrpc.CodeNotFound, "revision not found")
	}

	return res, nil
}

func graphGuideMapLayout() *graphv1.GuideMapLayout {
	return &graphv1.GuideMapLayout{
		Groups: []*graphv1.GuideMapGroup{{GroupId: "g-1f"}, {GroupId: "g-2f"}},
		Objects: []*graphv1.GuideMapObject{
			{
				ObjectId: "p-hall", GroupId: "g-1f", Color: "indigo", PointId: "p-hall", Closed: true,
				Title:       &graphv1.LocalizedText{Texts: map[string]string{"ja": "大ホール", "en": "Main hall"}},
				Description: &graphv1.LocalizedText{Texts: map[string]string{"ja": "2 階まで吹き抜け"}},
				Kind: &graphv1.GuideMapObject_Place{Place: &graphv1.GuideMapPlace{
					Center: &graphv1.GuideMapPosition{X: 1, Y: 2}, Shape: graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT,
					Width: 30, Height: 40, Pin: true,
				}},
			},
			{
				ObjectId: "c-stairs", GroupId: "g-1f", Color: "indigo", PointId: "p-stairs",
				Kind: &graphv1.GuideMapObject_Place{Place: &graphv1.GuideMapPlace{
					Center: &graphv1.GuideMapPosition{X: 5, Y: 6}, Shape: graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN,
					MarkWidth: proto.Float64(40), MarkHeight: proto.Float64(30),
					Connector: &graphv1.GuideMapConnector{
						Type: graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR, Link: "l-1",
						Direction: graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP,
					},
				}},
			},
			{
				ObjectId: "l-corridor", GroupId: "g-1f", Color: "slate", RouteId: "r-corridor",
				Kind: &graphv1.GuideMapObject_Line{Line: &graphv1.GuideMapLine{
					Points: []*graphv1.GuideMapPosition{{X: 0, Y: 0}, {X: 3, Y: 4}}, Width: 6, Dash: true,
				}},
			},
			{
				ObjectId: "a-lobby", GroupId: "g-2f", Color: "rose",
				Kind: &graphv1.GuideMapObject_Area{Area: &graphv1.GuideMapArea{Points: []*graphv1.GuideMapPosition{{X: 7, Y: 8}}}},
			},
			{
				ObjectId: "t-title", GroupId: "g-2f", Color: "slate",
				Title: &graphv1.LocalizedText{Texts: map[string]string{"ja": "本館2階"}},
				Kind:  &graphv1.GuideMapObject_Text{Text: &graphv1.GuideMapText{Position: &graphv1.GuideMapPosition{X: 9, Y: 10}, Size: 22}},
			},
		},
		North:         90,
		Scale:         0.05,
		StartObjectId: "p-hall",
	}
}

func guestGuideMapLayout() *guestv1.GuideMapLayout {
	return &guestv1.GuideMapLayout{
		Groups: []*guestv1.GuideMapGroup{{GroupId: "g-1f"}, {GroupId: "g-2f"}},
		Objects: []*guestv1.GuideMapObject{
			{
				ObjectId: "p-hall", GroupId: "g-1f", Color: "indigo", PointId: "p-hall", Closed: true,
				Title:       map[string]string{"ja": "大ホール", "en": "Main hall"},
				Description: map[string]string{"ja": "2 階まで吹き抜け"},
				Kind: &guestv1.GuideMapObject_Place{Place: &guestv1.GuideMapPlace{
					Center: &guestv1.GuideMapPosition{X: 1, Y: 2}, Shape: guestv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT,
					Width: 30, Height: 40, Pin: true,
				}},
			},
			{
				ObjectId: "c-stairs", GroupId: "g-1f", Color: "indigo", PointId: "p-stairs",
				Kind: &guestv1.GuideMapObject_Place{Place: &guestv1.GuideMapPlace{
					Center: &guestv1.GuideMapPosition{X: 5, Y: 6}, Shape: guestv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN,
					MarkWidth: proto.Float64(40), MarkHeight: proto.Float64(30),
					Connector: &guestv1.GuideMapConnector{
						Type: guestv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR, Link: "l-1",
						Direction: guestv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP,
					},
				}},
			},
			{
				ObjectId: "l-corridor", GroupId: "g-1f", Color: "slate", RouteId: "r-corridor",
				Kind: &guestv1.GuideMapObject_Line{Line: &guestv1.GuideMapLine{
					Points: []*guestv1.GuideMapPosition{{X: 0, Y: 0}, {X: 3, Y: 4}}, Width: 6, Dash: true,
				}},
			},
			{
				ObjectId: "a-lobby", GroupId: "g-2f", Color: "rose",
				Kind: &guestv1.GuideMapObject_Area{Area: &guestv1.GuideMapArea{Points: []*guestv1.GuideMapPosition{{X: 7, Y: 8}}}},
			},
			{
				ObjectId: "t-title", GroupId: "g-2f", Color: "slate",
				Title: map[string]string{"ja": "本館2階"},
				Kind:  &guestv1.GuideMapObject_Text{Text: &guestv1.GuideMapText{Position: &guestv1.GuideMapPosition{X: 9, Y: 10}, Size: 22}},
			},
		},
		North:         90,
		Scale:         0.05,
		StartObjectId: "p-hall",
	}
}

func newGuideMapClient(t *testing.T) guestv1connect.GuestPageServiceClient {
	t.Helper()

	ctx := context.Background()
	supply := &fakeGuideMapSupply{responses: map[string]*graphv1.GetGuideMapLayoutResponse{
		"rev-1": {
			RevisionId:     "rev-1",
			GuideMapLayout: graphGuideMapLayout(),
			PointLabels: map[string]*graphv1.LocalizedText{
				"p-hall":   {Texts: map[string]string{"ja": "ホール", "en": "Hall"}},
				"p-stairs": {Texts: map[string]string{"ja": "階段"}},
			},
			GroupLabels: map[string]*graphv1.LocalizedText{
				"g-1f": {Texts: map[string]string{"ja": "1階", "en": "1F"}},
				"g-2f": {Texts: map[string]string{"ja": "2階"}},
			},
		},
		"rev-without-layout": {RevisionId: "rev-without-layout"},
	}}
	upstream := NewGraphAuthoringClient(http.DefaultClient, newGatewayServer(t, func(server *connectrpc.Server) {
		graphv1connect.RegisterGraphSupplyServiceHandler(server, supply)
	}))

	guideMap, found, err := upstream.GetGuideMap(ctx, testTenantPublicID, testEventPublicID, "rev-1")
	if err != nil || !found || guideMap == nil {
		t.Fatalf("GetGuideMap() = %v, %v, %v, want the guide map of the revision", guideMap, found, err)
	}

	if withoutLayout, found, err := upstream.GetGuideMap(ctx, testTenantPublicID, testEventPublicID, "rev-without-layout"); err != nil || !found || withoutLayout != nil {
		t.Errorf("GetGuideMap() of a revision without a layout = %v, %v, %v, want no guide map", withoutLayout, found, err)
	}

	if _, found, err := upstream.GetGuideMap(ctx, testTenantPublicID, testEventPublicID, "rev-gone"); err != nil || found {
		t.Errorf("GetGuideMap() of an unpublished revision = _, %v, %v, want not found without error", found, err)
	}

	readModel := newMemoryReadModel()
	_ = readModel.SaveGuideMap(ctx, *guideMap)
	_ = readModel.SaveQrLocations(ctx, domain.QrLocations{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Locations: []domain.QrLocation{
			{QrLocationID: "qr-hall", Name: "ホール前", Kind: "poster", PointID: "p-hall"},
			{QrLocationID: "qr-gate", Name: "門", Kind: "poster", PointID: "p-gate"},
			{QrLocationID: "qr-corridor", Name: "廊下", Kind: "poster", PointID: ""},
		},
	})

	routes, err := RoutesWithVerifier(newTestVerifier(t, internaljwt.JWKS{}), newGuestSettingsUseCases(), newGuestPageUseCases(readModel))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return guestv1connect.NewGuestPageServiceClient(newClient(newTestServer(t, mux)))
}

func TestGetGuideMap(t *testing.T) {
	t.Parallel()

	client := newGuideMapClient(t)
	ctx := context.Background()

	t.Run("returns the layout and only the labels of what it refers to, starting at the point of the QR location", func(t *testing.T) {
		t.Parallel()

		got, err := client.GetGuideMap(ctx, &guestv1.GetGuideMapRequest{EventId: testEventPublicID, QrLocationId: "qr-hall"})
		if err != nil {
			t.Fatalf("GetGuideMap() error = %v", err)
		}

		want := &guestv1.GetGuideMapResponse{
			Graph: &guestv1.GuideMapGraph{
				Points: []*guestv1.GuideMapPointLabels{
					{PointId: "p-hall", Labels: map[string]string{"ja": "ホール", "en": "Hall"}},
					{PointId: "p-stairs", Labels: map[string]string{"ja": "階段"}},
				},
				Groups: []*guestv1.GuideMapGroupLabels{
					{GroupId: "g-1f", Labels: map[string]string{"ja": "1階", "en": "1F"}},
					{GroupId: "g-2f", Labels: map[string]string{"ja": "2階"}},
				},
			},
			Layout:         guestGuideMapLayout(),
			CurrentPointId: "p-hall",
		}
		if !proto.Equal(got, want) {
			t.Errorf("GetGuideMap() = %v, want %v", got, want)
		}
	})

	t.Run("has no current point when the QR location is not at a point of the guide map", func(t *testing.T) {
		t.Parallel()

		for _, qrLocationID := range []string{"qr-gate", "qr-corridor", "qr-unknown", ""} {
			got, err := client.GetGuideMap(ctx, &guestv1.GetGuideMapRequest{EventId: testEventPublicID, QrLocationId: qrLocationID})
			if err != nil {
				t.Fatalf("GetGuideMap(%q) error = %v", qrLocationID, err)
			}

			if got.GetCurrentPointId() != "" || !proto.Equal(got.GetLayout(), guestGuideMapLayout()) {
				t.Errorf("GetGuideMap(%q) current point = %q, want none with the layout", qrLocationID, got.GetCurrentPointId())
			}
		}
	})

	t.Run("an event without a guide map is not found", func(t *testing.T) {
		t.Parallel()

		_, err := client.GetGuideMap(ctx, &guestv1.GetGuideMapRequest{EventId: otherEventPublicID, QrLocationId: "qr-hall"})
		assertConnectError(t, err, connectrpc.CodeNotFound, "guide map not found")

		_, err = client.GetGuideMap(ctx, &guestv1.GetGuideMapRequest{})
		assertConnectError(t, err, connectrpc.CodeInvalidArgument, "event ID is required")
	})
}

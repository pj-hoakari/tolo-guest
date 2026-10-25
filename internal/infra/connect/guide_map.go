package connect

import (
	"context"
	"maps"
	"slices"

	"google.golang.org/protobuf/proto"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"

	guestv1 "github.com/pj-hoakari/tolo-guest/gen/tolo/guest/v1"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

var guideMapPlaceShapes = map[graphv1.GuideMapPlaceShape]guestv1.GuideMapPlaceShape{
	graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN:    guestv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN,
	graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT:   guestv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT,
	graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_CIRCLE: guestv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_CIRCLE,
}

var guideMapConnectorTypes = map[graphv1.GuideMapConnectorType]guestv1.GuideMapConnectorType{
	graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_STAIRS:    guestv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_STAIRS,
	graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR: guestv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR,
	graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ELEVATOR:  guestv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ELEVATOR,
}

var guideMapConnectorDirections = map[graphv1.GuideMapConnectorDirection]guestv1.GuideMapConnectorDirection{
	graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_BOTH: guestv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_BOTH,
	graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP:   guestv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP,
	graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_DOWN: guestv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_DOWN,
}

func (s *GuestPageService) GetGuideMap(ctx context.Context, req *guestv1.GetGuideMapRequest) (*guestv1.GetGuideMapResponse, error) {
	view, err := s.useCases.GetGuideMap(ctx, req.GetEventId(), req.GetQrLocationId())
	if err != nil {
		return nil, connectError(ctx, err)
	}

	layout := new(guestv1.GuideMapLayout)
	if err := proto.Unmarshal(view.GuideMap.EncodedLayout, layout); err != nil {
		return nil, InternalError(ctx, err)
	}

	return &guestv1.GetGuideMapResponse{
		Graph:          guideMapGraph(view.GuideMap),
		Layout:         layout,
		CurrentPointId: view.CurrentPointID,
	}, nil
}

func guideMapGraph(guideMap domain.GuideMap) *guestv1.GuideMapGraph {
	points := make([]*guestv1.GuideMapPointLabels, 0, len(guideMap.Points))
	for _, pointID := range slices.Sorted(maps.Keys(guideMap.Points)) {
		points = append(points, &guestv1.GuideMapPointLabels{PointId: pointID, Labels: guideMap.Points[pointID]})
	}

	groups := make([]*guestv1.GuideMapGroupLabels, 0, len(guideMap.Groups))
	for _, groupID := range slices.Sorted(maps.Keys(guideMap.Groups)) {
		groups = append(groups, &guestv1.GuideMapGroupLabels{GroupId: groupID, Labels: guideMap.Groups[groupID]})
	}

	return &guestv1.GuideMapGraph{Points: points, Groups: groups}
}

func guideMapLayoutFromGraph(layout *graphv1.GuideMapLayout) *guestv1.GuideMapLayout {
	groups := make([]*guestv1.GuideMapGroup, 0, len(layout.GetGroups()))
	for _, group := range layout.GetGroups() {
		groups = append(groups, &guestv1.GuideMapGroup{GroupId: group.GetGroupId()})
	}

	objects := make([]*guestv1.GuideMapObject, 0, len(layout.GetObjects()))
	for _, object := range layout.GetObjects() {
		objects = append(objects, guideMapObjectFromGraph(object))
	}

	return &guestv1.GuideMapLayout{
		Groups:        groups,
		Objects:       objects,
		North:         layout.GetNorth(),
		Scale:         layout.GetScale(),
		StartObjectId: layout.GetStartObjectId(),
	}
}

func guideMapObjectFromGraph(object *graphv1.GuideMapObject) *guestv1.GuideMapObject {
	converted := &guestv1.GuideMapObject{
		ObjectId:    object.GetObjectId(),
		GroupId:     object.GetGroupId(),
		Title:       object.GetTitle().GetTexts(),
		Description: object.GetDescription().GetTexts(),
		Color:       object.GetColor(),
		Closed:      object.GetClosed(),
		PointId:     object.GetPointId(),
		RouteId:     object.GetRouteId(),
		Kind:        nil,
	}

	switch kind := object.GetKind().(type) {
	case *graphv1.GuideMapObject_Place:
		converted.Kind = &guestv1.GuideMapObject_Place{Place: guideMapPlaceFromGraph(kind.Place)}
	case *graphv1.GuideMapObject_Line:
		converted.Kind = &guestv1.GuideMapObject_Line{Line: &guestv1.GuideMapLine{
			Points: guideMapPositionsFromGraph(kind.Line.GetPoints()),
			Width:  kind.Line.GetWidth(),
			Dash:   kind.Line.GetDash(),
		}}
	case *graphv1.GuideMapObject_Area:
		converted.Kind = &guestv1.GuideMapObject_Area{Area: &guestv1.GuideMapArea{Points: guideMapPositionsFromGraph(kind.Area.GetPoints())}}
	case *graphv1.GuideMapObject_Text:
		converted.Kind = &guestv1.GuideMapObject_Text{Text: &guestv1.GuideMapText{
			Position: guideMapPositionFromGraph(kind.Text.GetPosition()),
			Size:     kind.Text.GetSize(),
		}}
	}

	return converted
}

func guideMapPlaceFromGraph(place *graphv1.GuideMapPlace) *guestv1.GuideMapPlace {
	converted := &guestv1.GuideMapPlace{
		Center:     guideMapPositionFromGraph(place.GetCenter()),
		Shape:      guideMapPlaceShapes[place.GetShape()],
		Width:      place.GetWidth(),
		Height:     place.GetHeight(),
		Pin:        place.GetPin(),
		Connector:  nil,
		MarkWidth:  place.MarkWidth,
		MarkHeight: place.MarkHeight,
	}

	if connector := place.GetConnector(); connector != nil {
		converted.Connector = &guestv1.GuideMapConnector{
			Type:      guideMapConnectorTypes[connector.GetType()],
			Link:      connector.GetLink(),
			Direction: guideMapConnectorDirections[connector.GetDirection()],
		}
	}

	return converted
}

func guideMapPositionFromGraph(position *graphv1.GuideMapPosition) *guestv1.GuideMapPosition {
	return &guestv1.GuideMapPosition{X: position.GetX(), Y: position.GetY()}
}

func guideMapPositionsFromGraph(positions []*graphv1.GuideMapPosition) []*guestv1.GuideMapPosition {
	converted := make([]*guestv1.GuideMapPosition, 0, len(positions))
	for _, position := range positions {
		converted = append(converted, guideMapPositionFromGraph(position))
	}

	return converted
}

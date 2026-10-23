package domain

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"
)

const DefaultLanguage = "ja"

var ErrEventIDRequired = errors.New("event ID is required")

type LocalizedText map[string]string

type RouteEndpoints struct {
	FromPointID string
	ToPointID   string
}

type DisplayNames struct {
	EventID        string
	RevisionID     string
	Points         map[string]LocalizedText
	Routes         map[string]string
	RouteEndpoints map[string]RouteEndpoints
}

type NamedCrowdStatus struct {
	CrowdStatus

	DisplayName string
}

type CrowdView struct {
	TakenAt time.Time
	Crowd   []NamedCrowdStatus
}

type QueueGate struct {
	PointID     string
	DisplayName string
	Recommended bool
}

type NamedQueueStatus struct {
	QueueID         string
	DisplayName     string
	WaitTimeMinutes float64
	TurnaroundCount int32
	Gates           []QueueGate
}

type QueuesView struct {
	TakenAt time.Time
	Queues  []NamedQueueStatus
}

func (t LocalizedText) In(language string) string {
	if text, ok := t[language]; ok {
		return text
	}

	if base, _, found := strings.Cut(language, "-"); found {
		if text, ok := t[base]; ok {
			return text
		}
	}

	if text, ok := t[DefaultLanguage]; ok {
		return text
	}

	if len(t) == 0 {
		return ""
	}

	return t[slices.Min(slices.Collect(maps.Keys(t)))]
}

func (n DisplayNames) NameCrowd(status GuestStatus, language string) (CrowdView, bool) {
	complete := true
	crowd := make([]NamedCrowdStatus, 0, len(status.Crowd))

	for _, c := range status.Crowd {
		name, known := n.targetName(c.Target, language)
		complete = complete && known

		crowd = append(crowd, NamedCrowdStatus{CrowdStatus: c, DisplayName: name})
	}

	return CrowdView{TakenAt: status.TakenAt, Crowd: crowd}, complete
}

func (n DisplayNames) NameQueues(status GuestStatus, language string) (QueuesView, bool) {
	complete := true
	queues := make([]NamedQueueStatus, 0, len(status.Queues))

	for _, q := range status.Queues {
		gates := make([]QueueGate, 0, len(q.GatePointIDs))
		names := make([]string, 0, len(q.GatePointIDs))

		for _, pointID := range q.GatePointIDs {
			name, known := n.pointName(pointID, language)
			complete = complete && known

			if name != "" {
				names = append(names, name)
			}

			gates = append(gates, QueueGate{
				PointID:     pointID,
				DisplayName: name,
				Recommended: slices.Contains(q.RecommendedGatePointIDs, pointID),
			})
		}

		queues = append(queues, NamedQueueStatus{
			QueueID:         q.QueueID,
			DisplayName:     strings.Join(names, " / "),
			WaitTimeMinutes: q.WaitTimeMinutes,
			TurnaroundCount: q.TurnaroundCount,
			Gates:           gates,
		})
	}

	return QueuesView{TakenAt: status.TakenAt, Queues: queues}, complete
}

func (n DisplayNames) targetName(target CrowdTarget, language string) (string, bool) {
	switch target.Kind {
	case CrowdTargetPoint:
		return n.pointName(target.ID, language)
	case CrowdTargetRoute:
		return n.routeName(target.ID, language)
	case CrowdTargetUnspecified:
	}

	return "", true
}

func (n DisplayNames) pointName(pointID, language string) (string, bool) {
	text, ok := n.Points[pointID]

	return text.In(language), ok
}

func (n DisplayNames) routeName(routeID, language string) (string, bool) {
	name, named := n.Routes[routeID]
	if name != "" {
		return name, true
	}

	endpoints, ok := n.RouteEndpoints[routeID]
	if !ok {
		return "", named
	}

	from, _ := n.pointName(endpoints.FromPointID, language)
	to, _ := n.pointName(endpoints.ToPointID, language)

	if from == "" || to == "" {
		return "", true
	}

	return from + " → " + to, true
}

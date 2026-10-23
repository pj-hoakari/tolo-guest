package domain

import "time"

type CrowdLevel int

const (
	CrowdLevelUnspecified CrowdLevel = iota
	CrowdLevelLow
	CrowdLevelMid
	CrowdLevelHigh
)

type CrowdTargetKind int

const (
	CrowdTargetUnspecified CrowdTargetKind = iota
	CrowdTargetPoint
	CrowdTargetRoute
)

type CrowdTarget struct {
	Kind CrowdTargetKind
	ID   string
}

type CrowdStatus struct {
	Target CrowdTarget
	Level  CrowdLevel
	Danger bool
}

type QueueStatus struct {
	QueueID                 string
	GatePointIDs            []string
	WaitTimeMinutes         float64
	TurnaroundCount         int32
	RecommendedGatePointIDs []string
}

type GuestStatus struct {
	EventID  string
	Sequence uint64
	TakenAt  time.Time
	Crowd    []CrowdStatus
	Queues   []QueueStatus
}

type GuestMessage struct {
	MessageID string
	Body      string
	SentAt    time.Time
}

type GuestMessagesUpdate struct {
	EventID           string
	Upserts           []GuestMessage
	RevokedMessageIDs []string
}

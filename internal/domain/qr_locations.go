package domain

type QrLocation struct {
	QrLocationID string
	Name         string
	Kind         string
}

type QrLocations struct {
	EventID    string
	RevisionID string
	Locations  []QrLocation
}

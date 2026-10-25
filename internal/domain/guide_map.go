package domain

type GuideMap struct {
	EventID       string
	RevisionID    string
	EncodedLayout []byte
	Points        map[string]LocalizedText
	Groups        map[string]LocalizedText
}

func (m GuideMap) CurrentPointID(location QrLocation) string {
	if _, ok := m.Points[location.PointID]; ok && location.PointID != "" {
		return location.PointID
	}

	return ""
}

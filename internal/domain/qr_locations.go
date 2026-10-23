package domain

import "net/url"

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

type IssuedURL struct {
	QrLocationID string
	Name         string
	Kind         string
	Path         string
}

func (l QrLocations) IssuedURLs() []IssuedURL {
	issued := make([]IssuedURL, 0, len(l.Locations))
	for _, location := range l.Locations {
		issued = append(issued, IssuedURL{
			QrLocationID: location.QrLocationID,
			Name:         location.Name,
			Kind:         location.Kind,
			Path:         "/event/" + url.PathEscape(l.EventID) + "/guest/" + url.PathEscape(location.QrLocationID),
		})
	}

	return issued
}

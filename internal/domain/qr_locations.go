package domain

import (
	"errors"
	"net/url"
)

var ErrQrLocationIDRequired = errors.New("QR location ID is required")

type QrLocation struct {
	QrLocationID string
	Name         string
	Kind         string
	PointID      string
}

type QrLocations struct {
	EventID    string
	RevisionID string
	Locations  []QrLocation
}

type QrAccess struct {
	EventID        string
	QrLocationID   string
	TenantPublicID string
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

func (l QrLocations) Find(qrLocationID string) (QrLocation, bool) {
	for _, location := range l.Locations {
		if location.QrLocationID == qrLocationID {
			return location, true
		}
	}

	var none QrLocation

	return none, false
}

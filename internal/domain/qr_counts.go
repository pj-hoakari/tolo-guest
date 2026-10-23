package domain

import (
	"cmp"
	"slices"
	"time"
)

const QrCountWindow = time.Minute

type RecordedQrAccess struct {
	ID       int64
	Access   QrAccess
	OpenedAt time.Time
}

type QrCount struct {
	QrLocationID string
	WindowStart  time.Time
	WindowEnd    time.Time
	Count        int32
	AccessIDs    []int64
}

type QrCountReport struct {
	EventID        string
	TenantPublicID string
	Counts         []QrCount
}

func GroupQrCounts(accesses []RecordedQrAccess) []QrCountReport {
	type reportKey struct{ eventID, tenantPublicID string }

	type countKey struct {
		report       reportKey
		qrLocationID string
		windowStart  time.Time
	}

	counts := map[countKey]*QrCount{}

	for _, access := range accesses {
		windowStart := access.OpenedAt.UTC().Truncate(QrCountWindow)
		key := countKey{
			report:       reportKey{eventID: access.Access.EventID, tenantPublicID: access.Access.TenantPublicID},
			qrLocationID: access.Access.QrLocationID,
			windowStart:  windowStart,
		}

		count, ok := counts[key]
		if !ok {
			count = &QrCount{
				QrLocationID: key.qrLocationID,
				WindowStart:  windowStart,
				WindowEnd:    windowStart.Add(QrCountWindow),
				Count:        0,
				AccessIDs:    nil,
			}
			counts[key] = count
		}

		count.Count++
		count.AccessIDs = append(count.AccessIDs, access.ID)
	}

	grouped := map[reportKey][]QrCount{}
	for key, count := range counts {
		grouped[key.report] = append(grouped[key.report], *count)
	}

	reports := make([]QrCountReport, 0, len(grouped))

	for key, reportCounts := range grouped {
		slices.SortFunc(reportCounts, func(a, b QrCount) int {
			return cmp.Or(cmp.Compare(a.QrLocationID, b.QrLocationID), a.WindowStart.Compare(b.WindowStart))
		})

		reports = append(reports, QrCountReport{EventID: key.eventID, TenantPublicID: key.tenantPublicID, Counts: reportCounts})
	}

	slices.SortFunc(reports, func(a, b QrCountReport) int {
		return cmp.Or(cmp.Compare(a.EventID, b.EventID), cmp.Compare(a.TenantPublicID, b.TenantPublicID))
	})

	return reports
}

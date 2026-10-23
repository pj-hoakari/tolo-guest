package application

//go:generate go tool mockgen -source=qr_count_report.go -destination=mock_qr_count_sink_test.go -package=application_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

var ErrQrCountsRefused = errors.New("QR counts refused")

type QrCountSink interface {
	ReportQrCounts(ctx context.Context, tenantPublicID, eventID string, counts []domain.QrCount) error
}

type QrCountReporter struct {
	accesses    repository.UnreportedQrAccessRepository
	observation QrCountSink
}

func NewQrCountReporter(accesses repository.UnreportedQrAccessRepository, observation QrCountSink) *QrCountReporter {
	return &QrCountReporter{accesses: accesses, observation: observation}
}

func (r *QrCountReporter) RunQrCountReports(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.ReportQrCounts(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "report QR counts failed", "error", err)
			}
		}
	}
}

func (r *QrCountReporter) ReportQrCounts(ctx context.Context) error {
	accesses, err := r.accesses.ListUnreportedQrAccesses(ctx, time.Now().Truncate(domain.QrCountWindow))
	if err != nil {
		return fmt.Errorf("list unreported QR accesses: %w", err)
	}

	var errs []error

	for _, report := range domain.GroupQrCounts(accesses) {
		done, err := r.send(ctx, report.TenantPublicID, report.EventID, report.Counts)
		if err != nil {
			errs = append(errs, fmt.Errorf("report QR counts of event %s: %w", report.EventID, err))
		}

		if len(done) == 0 {
			continue
		}

		if err := r.accesses.DeleteQrAccesses(ctx, done); err != nil {
			errs = append(errs, fmt.Errorf("delete reported QR accesses of event %s: %w", report.EventID, err))
		}
	}

	return errors.Join(errs...)
}

func (r *QrCountReporter) send(ctx context.Context, tenantPublicID, eventID string, counts []domain.QrCount) ([]int64, error) {
	err := r.observation.ReportQrCounts(ctx, tenantPublicID, eventID, counts)

	switch {
	case err == nil:
		return accessIDs(counts), nil
	case !errors.Is(err, ErrQrCountsRefused):
		return nil, err
	case len(counts) == 1:
		slog.WarnContext(ctx, "Observation refused QR counts, dropping them",
			"event_id", eventID, "qr_location_id", counts[0].QrLocationID,
			"window_start", counts[0].WindowStart, "count", counts[0].Count, "error", err)

		return counts[0].AccessIDs, nil
	}

	var (
		done []int64
		errs []error
	)

	for _, count := range counts {
		ids, err := r.send(ctx, tenantPublicID, eventID, []domain.QrCount{count})
		done = append(done, ids...)
		errs = append(errs, err)
	}

	return done, errors.Join(errs...)
}

func accessIDs(counts []domain.QrCount) []int64 {
	var ids []int64
	for _, count := range counts {
		ids = append(ids, count.AccessIDs...)
	}

	return ids
}

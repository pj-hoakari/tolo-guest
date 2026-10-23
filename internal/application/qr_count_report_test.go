package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

type qrCountMocks struct {
	accesses *MockUnreportedQrAccessRepository
	sink     *MockQrCountSink
}

func newQrCountReporter(t *testing.T) (*application.QrCountReporter, qrCountMocks) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mocks := qrCountMocks{accesses: NewMockUnreportedQrAccessRepository(ctrl), sink: NewMockQrCountSink(ctrl)}

	return application.NewQrCountReporter(mocks.accesses, mocks.sink), mocks
}

func recordedAccess(id int64, eventID, tenantPublicID, qrLocationID string, openedAt time.Time) domain.RecordedQrAccess {
	return domain.RecordedQrAccess{
		ID:       id,
		Access:   domain.QrAccess{EventID: eventID, QrLocationID: qrLocationID, TenantPublicID: tenantPublicID},
		OpenedAt: openedAt,
	}
}

var qrCountMinute = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

func qrCount(qrLocationID string, minute int, ids ...int64) domain.QrCount {
	start := qrCountMinute.Add(time.Duration(minute) * time.Minute)

	return domain.QrCount{
		QrLocationID: qrLocationID,
		WindowStart:  start,
		WindowEnd:    start.Add(time.Minute),
		Count:        int32(len(ids)),
		AccessIDs:    ids,
	}
}

func TestReportQrCountsListsOnlyClosedWindows(t *testing.T) {
	t.Parallel()

	reporter, mocks := newQrCountReporter(t)
	before := time.Now()

	mocks.accesses.EXPECT().ListUnreportedQrAccesses(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, openedBefore time.Time) ([]domain.RecordedQrAccess, error) {
			if !openedBefore.Equal(before.Truncate(time.Minute)) && !openedBefore.Equal(time.Now().Truncate(time.Minute)) {
				t.Errorf("ListUnreportedQrAccesses() opened before %v, want the start of the current minute", openedBefore)
			}

			return nil, nil
		})

	if err := reporter.ReportQrCounts(context.Background()); err != nil {
		t.Errorf("ReportQrCounts() error = %v", err)
	}
}

func TestReportQrCountsSendsOneRequestPerEventWithOneCountPerLocationAndMinute(t *testing.T) {
	t.Parallel()

	reporter, mocks := newQrCountReporter(t)
	at := func(minute, second int) time.Time {
		return qrCountMinute.Add(time.Duration(minute)*time.Minute + time.Duration(second)*time.Second)
	}

	mocks.accesses.EXPECT().ListUnreportedQrAccesses(gomock.Any(), gomock.Any()).Return([]domain.RecordedQrAccess{
		recordedAccess(1, testEventPublicID, testTenantPublicID, "qr-east", at(0, 5)),
		recordedAccess(2, otherEventPublicID, "other-tenant", "qr-gate", at(0, 10)),
		recordedAccess(3, testEventPublicID, testTenantPublicID, "qr-hall", at(0, 20)),
		recordedAccess(4, testEventPublicID, testTenantPublicID, "qr-east", at(0, 59)),
		recordedAccess(5, testEventPublicID, testTenantPublicID, "qr-east", at(1, 0)),
	}, nil)

	gomock.InOrder(
		mocks.sink.EXPECT().ReportQrCounts(gomock.Any(), testTenantPublicID, testEventPublicID, []domain.QrCount{
			qrCount("qr-east", 0, 1, 4),
			qrCount("qr-east", 1, 5),
			qrCount("qr-hall", 0, 3),
		}).Return(nil),
		mocks.accesses.EXPECT().DeleteQrAccesses(gomock.Any(), []int64{1, 4, 5, 3}).Return(nil),
		mocks.sink.EXPECT().ReportQrCounts(gomock.Any(), "other-tenant", otherEventPublicID, []domain.QrCount{
			qrCount("qr-gate", 0, 2),
		}).Return(nil),
		mocks.accesses.EXPECT().DeleteQrAccesses(gomock.Any(), []int64{2}).Return(nil),
	)

	if err := reporter.ReportQrCounts(context.Background()); err != nil {
		t.Errorf("ReportQrCounts() error = %v", err)
	}
}

func TestReportQrCountsKeepsTheAccessesOfAFailedSend(t *testing.T) {
	t.Parallel()

	reporter, mocks := newQrCountReporter(t)
	unavailable := errors.New("observation unavailable")

	mocks.accesses.EXPECT().ListUnreportedQrAccesses(gomock.Any(), gomock.Any()).Return([]domain.RecordedQrAccess{
		recordedAccess(1, testEventPublicID, testTenantPublicID, "qr-east", qrCountMinute),
		recordedAccess(2, otherEventPublicID, "other-tenant", "qr-gate", qrCountMinute),
	}, nil)
	mocks.sink.EXPECT().ReportQrCounts(gomock.Any(), testTenantPublicID, testEventPublicID, gomock.Any()).Return(unavailable)
	mocks.sink.EXPECT().ReportQrCounts(gomock.Any(), "other-tenant", otherEventPublicID, gomock.Any()).Return(nil)
	mocks.accesses.EXPECT().DeleteQrAccesses(gomock.Any(), []int64{2}).Return(nil)

	if err := reporter.ReportQrCounts(context.Background()); !errors.Is(err, unavailable) {
		t.Errorf("ReportQrCounts() error = %v, want %v", err, unavailable)
	}
}

func TestReportQrCountsDropsOnlyTheCountsObservationRefuses(t *testing.T) {
	t.Parallel()

	reporter, mocks := newQrCountReporter(t)
	refused := fmt.Errorf("permission denied: %w", application.ErrQrCountsRefused)

	mocks.accesses.EXPECT().ListUnreportedQrAccesses(gomock.Any(), gomock.Any()).Return([]domain.RecordedQrAccess{
		recordedAccess(1, testEventPublicID, testTenantPublicID, "qr-east", qrCountMinute),
		recordedAccess(2, testEventPublicID, testTenantPublicID, "qr-removed", qrCountMinute),
		recordedAccess(3, testEventPublicID, testTenantPublicID, "qr-west", qrCountMinute),
	}, nil)

	var sent [][]domain.QrCount

	mocks.sink.EXPECT().ReportQrCounts(gomock.Any(), testTenantPublicID, testEventPublicID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, counts []domain.QrCount) error {
			sent = append(sent, counts)

			for _, count := range counts {
				if count.QrLocationID == "qr-removed" {
					return refused
				}
			}

			return nil
		}).Times(4)
	mocks.accesses.EXPECT().DeleteQrAccesses(gomock.Any(), []int64{1, 2, 3}).Return(nil)

	if err := reporter.ReportQrCounts(context.Background()); err != nil {
		t.Errorf("ReportQrCounts() error = %v, want the refused counts dropped without error", err)
	}

	want := [][]domain.QrCount{
		{qrCount("qr-east", 0, 1), qrCount("qr-removed", 0, 2), qrCount("qr-west", 0, 3)},
		{qrCount("qr-east", 0, 1)},
		{qrCount("qr-removed", 0, 2)},
		{qrCount("qr-west", 0, 3)},
	}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("sent counts = %+v, want the whole event once and then each count alone", sent)
	}
}

func TestReportQrCountsSendsNothingWithoutAccesses(t *testing.T) {
	t.Parallel()

	reporter, mocks := newQrCountReporter(t)
	mocks.accesses.EXPECT().ListUnreportedQrAccesses(gomock.Any(), gomock.Any()).Return(nil, nil)

	if err := reporter.ReportQrCounts(context.Background()); err != nil {
		t.Errorf("ReportQrCounts() error = %v", err)
	}
}

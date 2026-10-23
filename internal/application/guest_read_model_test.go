package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

type readModelMocks struct {
	readModel *MockGuestReadModelRepository
	settings  *MockGuestSettingsRepository
	statuses  *MockGuestStatusSource
	messages  *MockGuestMessageSource
	names     *MockDisplayNameSource
	qr        *MockQrLocationSource
	accesses  *MockQrAccessRepository
}

func newGuestReadModelService(t *testing.T) (*application.GuestReadModelService, readModelMocks) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mocks := readModelMocks{
		readModel: NewMockGuestReadModelRepository(ctrl),
		settings:  NewMockGuestSettingsRepository(ctrl),
		statuses:  NewMockGuestStatusSource(ctrl),
		messages:  NewMockGuestMessageSource(ctrl),
		names:     NewMockDisplayNameSource(ctrl),
		qr:        NewMockQrLocationSource(ctrl),
		accesses:  NewMockQrAccessRepository(ctrl),
	}

	return application.NewGuestReadModelService(
		mocks.readModel, mocks.settings, mocks.statuses, mocks.messages, mocks.names, mocks.qr, mocks.accesses,
	), mocks
}

func TestApplyGuestPushes(t *testing.T) {
	t.Parallel()

	status := domain.GuestStatus{EventID: testEventPublicID, Sequence: 7, TakenAt: time.Now(), Crowd: nil, Queues: nil}
	update := domain.GuestMessagesUpdate{EventID: testEventPublicID, Upserts: nil, RevokedMessageIDs: []string{"m1"}}

	t.Run("applies a status and a messages update to an event without settings", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().ApplyStatus(gomock.Any(), status).Return(nil)
		mocks.readModel.EXPECT().ApplyMessages(gomock.Any(), update).Return(nil)

		if err := service.ApplyGuestStatus(context.Background(), status); err != nil {
			t.Errorf("ApplyGuestStatus() error = %v", err)
		}

		if err := service.ApplyGuestMessages(context.Background(), update); err != nil {
			t.Errorf("ApplyGuestMessages() error = %v", err)
		}
	})

	t.Run("reports a failed write so that the push is redelivered", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		down := errors.New("database down")
		mocks.readModel.EXPECT().ApplyStatus(gomock.Any(), status).Return(down)
		mocks.readModel.EXPECT().ApplyMessages(gomock.Any(), update).Return(down)

		if err := service.ApplyGuestStatus(context.Background(), status); !errors.Is(err, down) {
			t.Errorf("ApplyGuestStatus() error = %v, want %v", err, down)
		}

		if err := service.ApplyGuestMessages(context.Background(), update); !errors.Is(err, down) {
			t.Errorf("ApplyGuestMessages() error = %v, want %v", err, down)
		}
	})
}

func TestReconcile(t *testing.T) {
	t.Parallel()

	t.Run("pulls the snapshot and display names of every known event and the messages of events with a tenant", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		snapshot := domain.GuestStatus{EventID: testEventPublicID, Sequence: 3, TakenAt: time.Now(), Crowd: nil, Queues: nil}
		messages := []domain.GuestMessage{{MessageID: "m1", Body: "hello", SentAt: time.Now()}}

		mocks.readModel.EXPECT().ListEventIDs(gomock.Any()).Return([]string{otherEventPublicID, testEventPublicID}, nil)
		mocks.settings.EXPECT().ListEventTenants(gomock.Any()).
			Return([]domain.EventTenant{{EventID: testEventPublicID, TenantPublicID: testTenantPublicID}}, nil)

		mocks.statuses.EXPECT().GetGuestSnapshot(gomock.Any(), testEventPublicID).Return(snapshot, true, nil)
		mocks.statuses.EXPECT().GetGuestSnapshot(gomock.Any(), otherEventPublicID).Return(domain.GuestStatus{}, false, nil)
		mocks.readModel.EXPECT().ApplyStatus(gomock.Any(), snapshot).Return(nil)

		names := domain.DisplayNames{EventID: testEventPublicID, RevisionID: "rev-1", Points: nil, Routes: nil, RouteEndpoints: nil}
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), testEventPublicID).Return(names, true, nil)
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), otherEventPublicID).Return(domain.DisplayNames{}, false, nil)
		mocks.readModel.EXPECT().SaveDisplayNames(gomock.Any(), names).Return(nil)

		locations := domain.QrLocations{EventID: testEventPublicID, RevisionID: "rev-1", Locations: nil}
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(domain.QrLocations{}, repository.ErrQrLocationsNotFound)
		mocks.qr.EXPECT().GetQrLocations(gomock.Any(), testTenantPublicID, testEventPublicID, "rev-1").Return(locations, true, nil)
		mocks.readModel.EXPECT().SaveQrLocations(gomock.Any(), locations).Return(nil)

		mocks.messages.EXPECT().ListGuestMessages(gomock.Any(), testTenantPublicID, testEventPublicID).Return(messages, nil)
		mocks.readModel.EXPECT().ReplaceMessages(gomock.Any(), testEventPublicID, messages).Return(nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("pulls an event that only has settings", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().ListEventIDs(gomock.Any()).Return(nil, nil)
		mocks.settings.EXPECT().ListEventTenants(gomock.Any()).
			Return([]domain.EventTenant{{EventID: testEventPublicID, TenantPublicID: testTenantPublicID}}, nil)
		mocks.statuses.EXPECT().GetGuestSnapshot(gomock.Any(), testEventPublicID).Return(domain.GuestStatus{}, false, nil)
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), testEventPublicID).Return(domain.DisplayNames{}, false, nil)
		mocks.messages.EXPECT().ListGuestMessages(gomock.Any(), testTenantPublicID, testEventPublicID).Return(nil, nil)
		mocks.readModel.EXPECT().ReplaceMessages(gomock.Any(), testEventPublicID, nil).Return(nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("keeps pulling the other events after one fails", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		unavailable := errors.New("observation unavailable")
		snapshot := domain.GuestStatus{EventID: otherEventPublicID, Sequence: 1, TakenAt: time.Now(), Crowd: nil, Queues: nil}

		mocks.readModel.EXPECT().ListEventIDs(gomock.Any()).Return([]string{testEventPublicID, otherEventPublicID}, nil)
		mocks.settings.EXPECT().ListEventTenants(gomock.Any()).Return(nil, nil)
		mocks.statuses.EXPECT().GetGuestSnapshot(gomock.Any(), testEventPublicID).Return(domain.GuestStatus{}, false, unavailable)
		mocks.statuses.EXPECT().GetGuestSnapshot(gomock.Any(), otherEventPublicID).Return(snapshot, true, nil)
		mocks.readModel.EXPECT().ApplyStatus(gomock.Any(), snapshot).Return(nil)
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), testEventPublicID).Return(domain.DisplayNames{}, false, unavailable)
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), otherEventPublicID).Return(domain.DisplayNames{}, false, nil)

		if err := service.Reconcile(context.Background()); !errors.Is(err, unavailable) {
			t.Errorf("Reconcile() error = %v, want %v", err, unavailable)
		}
	})
}

func TestReconcileQrLocations(t *testing.T) {
	t.Parallel()

	stored := domain.QrLocations{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Locations:  []domain.QrLocation{{QrLocationID: "qr-east", Name: "東門", Kind: "entrance"}},
	}
	pulled := domain.QrLocations{
		EventID:    testEventPublicID,
		RevisionID: "rev-2",
		Locations:  []domain.QrLocation{{QrLocationID: "qr-west", Name: "西門", Kind: "entrance"}},
	}

	publishedAt := func(t *testing.T, revisionID string, tenants []domain.EventTenant) (*application.GuestReadModelService, readModelMocks) {
		t.Helper()

		service, mocks := newGuestReadModelService(t)
		names := domain.DisplayNames{EventID: testEventPublicID, RevisionID: revisionID, Points: nil, Routes: nil, RouteEndpoints: nil}

		mocks.readModel.EXPECT().ListEventIDs(gomock.Any()).Return([]string{testEventPublicID}, nil)
		mocks.settings.EXPECT().ListEventTenants(gomock.Any()).Return(tenants, nil)
		mocks.statuses.EXPECT().GetGuestSnapshot(gomock.Any(), testEventPublicID).Return(domain.GuestStatus{}, false, nil)
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), testEventPublicID).Return(names, true, nil)
		mocks.readModel.EXPECT().SaveDisplayNames(gomock.Any(), names).Return(nil)

		for _, tenant := range tenants {
			mocks.messages.EXPECT().ListGuestMessages(gomock.Any(), tenant.TenantPublicID, tenant.EventID).Return(nil, nil)
			mocks.readModel.EXPECT().ReplaceMessages(gomock.Any(), tenant.EventID, nil).Return(nil)
		}

		return service, mocks
	}

	withTenant := []domain.EventTenant{{EventID: testEventPublicID, TenantPublicID: testTenantPublicID}}

	t.Run("pulls the QR locations of the published revision when none are stored", func(t *testing.T) {
		t.Parallel()

		service, mocks := publishedAt(t, "rev-2", withTenant)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(domain.QrLocations{}, repository.ErrQrLocationsNotFound)
		mocks.qr.EXPECT().GetQrLocations(gomock.Any(), testTenantPublicID, testEventPublicID, "rev-2").Return(pulled, true, nil)
		mocks.readModel.EXPECT().SaveQrLocations(gomock.Any(), pulled).Return(nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("pulls the QR locations again when the published revision changes", func(t *testing.T) {
		t.Parallel()

		service, mocks := publishedAt(t, "rev-2", withTenant)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(stored, nil)
		mocks.qr.EXPECT().GetQrLocations(gomock.Any(), testTenantPublicID, testEventPublicID, "rev-2").Return(pulled, true, nil)
		mocks.readModel.EXPECT().SaveQrLocations(gomock.Any(), pulled).Return(nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("does not pull the QR locations while the published revision stays the same", func(t *testing.T) {
		t.Parallel()

		service, mocks := publishedAt(t, "rev-1", withTenant)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(stored, nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("does not pull the QR locations of an event whose tenant is unknown", func(t *testing.T) {
		t.Parallel()

		service, _ := publishedAt(t, "rev-2", nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("keeps the previous QR locations when the revision is no longer published", func(t *testing.T) {
		t.Parallel()

		service, mocks := publishedAt(t, "rev-2", withTenant)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(stored, nil)
		mocks.qr.EXPECT().GetQrLocations(gomock.Any(), testTenantPublicID, testEventPublicID, "rev-2").Return(domain.QrLocations{}, false, nil)

		if err := service.Reconcile(context.Background()); err != nil {
			t.Errorf("Reconcile() error = %v", err)
		}
	})

	t.Run("reports a failed pull and keeps the previous QR locations", func(t *testing.T) {
		t.Parallel()

		service, mocks := publishedAt(t, "rev-2", withTenant)
		unavailable := errors.New("graph authoring unavailable")

		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(stored, nil)
		mocks.qr.EXPECT().GetQrLocations(gomock.Any(), testTenantPublicID, testEventPublicID, "rev-2").Return(domain.QrLocations{}, false, unavailable)

		if err := service.Reconcile(context.Background()); !errors.Is(err, unavailable) {
			t.Errorf("Reconcile() error = %v, want %v", err, unavailable)
		}
	})
}

func TestRunReconciliation(t *testing.T) {
	t.Parallel()

	service, mocks := newGuestReadModelService(t)
	ctx, cancel := context.WithCancel(context.Background())
	pulled := make(chan struct{}, 2)

	mocks.readModel.EXPECT().ListEventIDs(gomock.Any()).DoAndReturn(func(context.Context) ([]string, error) {
		select {
		case pulled <- struct{}{}:
		default:
		}

		return nil, errors.New("database down")
	}).MinTimes(2)
	mocks.settings.EXPECT().ListEventTenants(gomock.Any()).Times(0)

	done := make(chan struct{})

	go func() {
		defer close(done)

		service.RunReconciliation(ctx, time.Millisecond)
	}()

	<-pulled
	<-pulled
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunReconciliation() did not return after its context was cancelled")
	}
}

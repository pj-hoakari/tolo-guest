package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-guest/internal/application"
	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/repository"
)

func pageStatus(eventID string) domain.GuestStatus {
	return domain.GuestStatus{
		EventID:  eventID,
		Sequence: 5,
		TakenAt:  time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC),
		Crowd: []domain.CrowdStatus{
			{Target: domain.CrowdTarget{Kind: domain.CrowdTargetPoint, ID: "p-east"}, Level: domain.CrowdLevelHigh, Danger: true},
		},
		Queues: []domain.QueueStatus{
			{QueueID: "q1", GatePointIDs: []string{"p-east"}, WaitTimeMinutes: 4, TurnaroundCount: 1, RecommendedGatePointIDs: nil},
		},
	}
}

func pageNames(eventID string) domain.DisplayNames {
	return domain.DisplayNames{
		EventID:        eventID,
		RevisionID:     "rev-1",
		Points:         map[string]domain.LocalizedText{"p-east": {"ja": "東門", "en": "East Gate"}},
		Routes:         nil,
		RouteEndpoints: nil,
	}
}

func TestGuestPageRejectsAMissingEventID(t *testing.T) {
	t.Parallel()

	service, _ := newGuestReadModelService(t)
	ctx := context.Background()

	if _, err := service.GetCrowd(ctx, "", "ja"); !errors.Is(err, domain.ErrEventIDRequired) {
		t.Errorf("GetCrowd() error = %v, want %v", err, domain.ErrEventIDRequired)
	}

	if _, err := service.GetQueues(ctx, "", "ja"); !errors.Is(err, domain.ErrEventIDRequired) {
		t.Errorf("GetQueues() error = %v, want %v", err, domain.ErrEventIDRequired)
	}

	if _, err := service.ListGuestMessages(ctx, ""); !errors.Is(err, domain.ErrEventIDRequired) {
		t.Errorf("ListGuestMessages() error = %v, want %v", err, domain.ErrEventIDRequired)
	}
}

func TestGuestPageServesAnEventWithoutStatusAsEmpty(t *testing.T) {
	t.Parallel()

	service, mocks := newGuestReadModelService(t)
	mocks.readModel.EXPECT().FindStatus(gomock.Any(), testEventPublicID).
		Return(domain.GuestStatus{}, repository.ErrGuestStatusNotFound).Times(2)

	crowd, err := service.GetCrowd(context.Background(), testEventPublicID, "ja")
	if err != nil || !crowd.TakenAt.IsZero() || len(crowd.Crowd) != 0 {
		t.Errorf("GetCrowd() = %+v, %v, want an empty view without taken_at", crowd, err)
	}

	queues, err := service.GetQueues(context.Background(), testEventPublicID, "ja")
	if err != nil || !queues.TakenAt.IsZero() || len(queues.Queues) != 0 {
		t.Errorf("GetQueues() = %+v, %v, want an empty view without taken_at", queues, err)
	}
}

func TestGuestPageNamesTheStatusFromTheStoredCopy(t *testing.T) {
	t.Parallel()

	service, mocks := newGuestReadModelService(t)
	mocks.readModel.EXPECT().FindStatus(gomock.Any(), testEventPublicID).Return(pageStatus(testEventPublicID), nil).Times(2)
	mocks.readModel.EXPECT().FindDisplayNames(gomock.Any(), testEventPublicID).Return(pageNames(testEventPublicID), nil).Times(2)

	crowd, err := service.GetCrowd(context.Background(), testEventPublicID, "en")
	if err != nil {
		t.Fatalf("GetCrowd() error = %v", err)
	}

	wantCrowd := domain.CrowdView{
		TakenAt: pageStatus(testEventPublicID).TakenAt,
		Crowd:   []domain.NamedCrowdStatus{{CrowdStatus: pageStatus(testEventPublicID).Crowd[0], DisplayName: "East Gate"}},
	}
	if !reflect.DeepEqual(crowd, wantCrowd) {
		t.Errorf("GetCrowd() = %+v, want %+v", crowd, wantCrowd)
	}

	queues, err := service.GetQueues(context.Background(), testEventPublicID, "ja")
	if err != nil {
		t.Fatalf("GetQueues() error = %v", err)
	}

	if got := queues.Queues[0].DisplayName; got != "東門" {
		t.Errorf("GetQueues() queue display name = %q, want %q", got, "東門")
	}
}

func TestGuestPageAnswersWithoutCallingGraphAuthoring(t *testing.T) {
	t.Parallel()

	service, mocks := newGuestReadModelService(t)
	mocks.readModel.EXPECT().FindStatus(gomock.Any(), testEventPublicID).Return(pageStatus(testEventPublicID), nil)
	mocks.readModel.EXPECT().FindDisplayNames(gomock.Any(), testEventPublicID).
		Return(domain.DisplayNames{}, repository.ErrDisplayNamesNotFound)

	crowd, err := service.GetCrowd(context.Background(), testEventPublicID, "ja")
	if err != nil {
		t.Fatalf("GetCrowd() error = %v", err)
	}

	if got := crowd.Crowd[0].DisplayName; got != "" {
		t.Errorf("GetCrowd() display name = %q, want none before the names are pulled", got)
	}
}

func TestGuestPageRefreshesDisplayNamesOfUnknownIDsInTheBackground(t *testing.T) {
	t.Parallel()

	service, mocks := newGuestReadModelService(t)
	ctx, cancel := context.WithCancel(context.Background())
	pulled := make(chan string, 2)

	mocks.readModel.EXPECT().ListEventIDs(gomock.Any()).Return(nil, nil)
	mocks.settings.EXPECT().ListEventTenants(gomock.Any()).Return(nil, nil)

	for _, eventID := range []string{testEventPublicID, otherEventPublicID} {
		mocks.readModel.EXPECT().FindStatus(gomock.Any(), eventID).Return(pageStatus(eventID), nil).AnyTimes()
		mocks.readModel.EXPECT().FindDisplayNames(gomock.Any(), eventID).
			Return(domain.DisplayNames{}, repository.ErrDisplayNamesNotFound).AnyTimes()
		mocks.names.EXPECT().GetDisplayNames(gomock.Any(), eventID).Return(pageNames(eventID), true, nil).Times(1)
		mocks.readModel.EXPECT().SaveDisplayNames(gomock.Any(), pageNames(eventID)).DoAndReturn(
			func(_ context.Context, names domain.DisplayNames) error {
				pulled <- names.EventID

				return nil
			})
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		service.RunReconciliation(ctx, time.Hour)
	}()

	if _, err := service.GetCrowd(ctx, testEventPublicID, "ja"); err != nil {
		t.Fatalf("GetCrowd() error = %v", err)
	}

	if got := <-pulled; got != testEventPublicID {
		t.Fatalf("pulled display names of %q, want %q", got, testEventPublicID)
	}

	if _, err := service.GetQueues(ctx, testEventPublicID, "ja"); err != nil {
		t.Fatalf("GetQueues() error = %v", err)
	}

	if _, err := service.GetCrowd(ctx, otherEventPublicID, "ja"); err != nil {
		t.Fatalf("GetCrowd() error = %v", err)
	}

	if got := <-pulled; got != otherEventPublicID {
		t.Errorf("pulled display names of %q, want only %q since %q was just refreshed", got, otherEventPublicID, testEventPublicID)
	}

	cancel()
	<-done
}

func TestListGuestMessages(t *testing.T) {
	t.Parallel()

	t.Run("returns the stored messages", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		messages := []domain.GuestMessage{{MessageID: "m1", Body: "Gate A is open", SentAt: time.Now()}}
		mocks.readModel.EXPECT().ListMessages(gomock.Any(), testEventPublicID).Return(messages, nil)

		got, err := service.ListGuestMessages(context.Background(), testEventPublicID)
		if err != nil || !reflect.DeepEqual(got, messages) {
			t.Errorf("ListGuestMessages() = %+v, %v, want %+v", got, err, messages)
		}
	})

	t.Run("reports a failed read", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		down := errors.New("database down")
		mocks.readModel.EXPECT().ListMessages(gomock.Any(), testEventPublicID).Return(nil, down)

		if _, err := service.ListGuestMessages(context.Background(), testEventPublicID); !errors.Is(err, down) {
			t.Errorf("ListGuestMessages() error = %v, want %v", err, down)
		}
	})
}

func TestOpenGuestPage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	locations := domain.QrLocations{
		EventID:    testEventPublicID,
		RevisionID: "rev-1",
		Locations: []domain.QrLocation{
			{QrLocationID: "qr-east", Name: "東門", Kind: "entrance"},
			{QrLocationID: "qr-hall", Name: "ホール前", Kind: "queue"},
		},
	}
	settings := domain.GuestSettings{
		EventID:        testEventPublicID,
		TenantPublicID: testTenantPublicID,
		PageCompositions: []domain.PageComposition{
			{Kind: "entrance", Components: []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd}},
		},
	}
	access := func(qrLocationID string) domain.QrAccess {
		return domain.QrAccess{EventID: testEventPublicID, QrLocationID: qrLocationID, TenantPublicID: testTenantPublicID}
	}

	open := func(t *testing.T, service *application.GuestReadModelService, qrLocationID, sessionID string, want []domain.Component) {
		t.Helper()

		got, err := service.OpenGuestPage(ctx, testEventPublicID, qrLocationID, sessionID)
		if err != nil {
			t.Fatalf("OpenGuestPage() error = %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("OpenGuestPage() = %v, want %v", got, want)
		}
	}

	t.Run("answers the composition of the location's kind and counts the open once", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil)
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(settings, nil)
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-east")).Return(nil)

		open(t, service, "qr-east", "session-a", []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd})
	})

	t.Run("counts a session's repeat opens of a location once within 30 minutes and still answers each open", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil).AnyTimes()
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(settings, nil).AnyTimes()
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-east")).Return(nil)

		composition := []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd}
		open(t, service, "qr-east", "session-a", composition)

		*mocks.now = mocks.now.Add(30*time.Minute - time.Nanosecond)

		open(t, service, "qr-east", "session-a", composition)

		*mocks.now = mocks.now.Add(time.Nanosecond)
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-east")).Return(nil)
		open(t, service, "qr-east", "session-a", composition)
	})

	t.Run("counts different sessions and different locations of one session separately", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil).AnyTimes()
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(settings, nil).AnyTimes()
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-east")).Return(nil).Times(2)
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-hall")).Return(nil)

		open(t, service, "qr-east", "session-a", []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd})
		open(t, service, "qr-east", "session-b", []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd})
		open(t, service, "qr-hall", "session-a", domain.DefaultComponents())
	})

	t.Run("answers the composition without counting an open without a session", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil).Times(2)
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(settings, nil).Times(2)

		open(t, service, "qr-east", "", []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd})
		open(t, service, "qr-east", "", []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd})
	})

	t.Run("answers the default composition for a kind without a composition and still counts", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil)
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(settings, nil)
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-hall")).Return(nil)

		open(t, service, "qr-hall", "session-a", domain.DefaultComponents())
	})

	t.Run("answers the default composition without counting for a location missing from the pulled list", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil)

		open(t, service, "qr-unknown", "session-a", domain.DefaultComponents())
	})

	t.Run("answers the default composition without counting for an event whose locations are not pulled", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(domain.QrLocations{}, repository.ErrQrLocationsNotFound)

		open(t, service, "qr-east", "session-a", domain.DefaultComponents())
	})

	t.Run("does not count a known location of an event whose tenant is unknown", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil)
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(domain.GuestSettings{}, repository.ErrGuestSettingsNotFound)

		open(t, service, "qr-east", "session-a", domain.DefaultComponents())
	})

	t.Run("still answers the composition when counting fails", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(locations, nil)
		mocks.settings.EXPECT().Find(gomock.Any(), testEventPublicID).Return(settings, nil)
		mocks.accesses.EXPECT().RecordQrAccess(gomock.Any(), access("qr-east")).Return(errors.New("database unavailable"))

		open(t, service, "qr-east", "session-a", []domain.Component{domain.ComponentStaffMessages, domain.ComponentCrowd})
	})

	t.Run("reports a failed read of the QR locations", func(t *testing.T) {
		t.Parallel()

		service, mocks := newGuestReadModelService(t)
		unavailable := errors.New("database unavailable")
		mocks.readModel.EXPECT().FindQrLocations(gomock.Any(), testEventPublicID).Return(domain.QrLocations{}, unavailable)

		if _, err := service.OpenGuestPage(ctx, testEventPublicID, "qr-east", "session-a"); !errors.Is(err, unavailable) {
			t.Errorf("OpenGuestPage() error = %v, want %v", err, unavailable)
		}
	})

	t.Run("rejects a missing event ID or QR location ID", func(t *testing.T) {
		t.Parallel()

		service, _ := newGuestReadModelService(t)

		if _, err := service.OpenGuestPage(ctx, "", "qr-east", "session-a"); !errors.Is(err, domain.ErrEventIDRequired) {
			t.Errorf("OpenGuestPage() error = %v, want %v", err, domain.ErrEventIDRequired)
		}

		if _, err := service.OpenGuestPage(ctx, testEventPublicID, "", "session-a"); !errors.Is(err, domain.ErrQrLocationIDRequired) {
			t.Errorf("OpenGuestPage() error = %v, want %v", err, domain.ErrQrLocationIDRequired)
		}
	})
}

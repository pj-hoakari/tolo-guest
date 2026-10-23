package db

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

func TestRecordQrAccess(t *testing.T) {
	t.Parallel()

	repo := NewPostgresQrAccessRepository(testDB)
	ctx := context.Background()
	access := domain.QrAccess{EventID: "qr-accesses-event", QrLocationID: "qr-east", TenantPublicID: "qr-accesses-tenant"}

	for range 2 {
		if err := repo.RecordQrAccess(ctx, access); err != nil {
			t.Fatalf("RecordQrAccess() error = %v", err)
		}
	}

	type row struct {
		ID             int64  `db:"id"`
		EventID        string `db:"event_id"`
		QrLocationID   string `db:"qr_location_id"`
		TenantPublicID string `db:"tenant_public_id"`
		HasOpenedAt    bool   `db:"has_opened_at"`
	}

	var rows []row
	if err := testDB.SelectContext(ctx, &rows, `
		SELECT id, event_id, qr_location_id, tenant_public_id,
			opened_at IS NOT NULL AS has_opened_at
		FROM guest_qr_accesses
		WHERE event_id = $1
		ORDER BY id`, access.EventID); err != nil {
		t.Fatalf("select QR accesses: %v", err)
	}

	if len(rows) != 2 || rows[0].ID == rows[1].ID {
		t.Fatalf("QR accesses = %+v, want one row per open", rows)
	}

	for _, got := range rows {
		want := row{ID: got.ID, EventID: access.EventID, QrLocationID: access.QrLocationID, TenantPublicID: access.TenantPublicID, HasOpenedAt: true}
		if got != want {
			t.Errorf("QR access = %+v, want %+v", got, want)
		}
	}
}

func TestListAndDeleteUnreportedQrAccesses(t *testing.T) {
	t.Parallel()

	repo := NewPostgresQrAccessRepository(testDB)
	ctx := context.Background()
	closedAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	tenant := "list-qr-accesses-tenant"

	insert := func(eventID string, openedAt time.Time) int64 {
		t.Helper()

		var id int64
		if err := testDB.GetContext(ctx, &id, `
			INSERT INTO guest_qr_accesses (event_id, qr_location_id, tenant_public_id, opened_at)
			VALUES ($1, 'qr-east', $2, $3)
			RETURNING id`, eventID, tenant, openedAt); err != nil {
			t.Fatalf("insert QR access: %v", err)
		}

		return id
	}

	first := insert("list-qr-accesses-a", closedAt.Add(-59*time.Second))
	second := insert("list-qr-accesses-b", closedAt.Add(-time.Microsecond))
	open := insert("list-qr-accesses-a", closedAt)

	listed, err := repo.ListUnreportedQrAccesses(ctx, closedAt)
	if err != nil {
		t.Fatalf("ListUnreportedQrAccesses() error = %v", err)
	}

	var got []domain.RecordedQrAccess

	for _, access := range listed {
		if access.Access.TenantPublicID == tenant {
			access.OpenedAt = access.OpenedAt.UTC()
			got = append(got, access)
		}
	}

	want := []domain.RecordedQrAccess{
		{ID: first, Access: domain.QrAccess{EventID: "list-qr-accesses-a", QrLocationID: "qr-east", TenantPublicID: tenant}, OpenedAt: closedAt.Add(-59 * time.Second)},
		{ID: second, Access: domain.QrAccess{EventID: "list-qr-accesses-b", QrLocationID: "qr-east", TenantPublicID: tenant}, OpenedAt: closedAt.Add(-time.Microsecond)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListUnreportedQrAccesses() = %+v, want the accesses opened before %v in id order", got, closedAt)
	}

	if err := repo.DeleteQrAccesses(ctx, []int64{first}); err != nil {
		t.Fatalf("DeleteQrAccesses() error = %v", err)
	}

	var remaining []int64
	if err := testDB.SelectContext(ctx, &remaining, `
		SELECT id FROM guest_qr_accesses WHERE tenant_public_id = $1 ORDER BY id`, tenant); err != nil {
		t.Fatalf("select remaining QR accesses: %v", err)
	}

	if !reflect.DeepEqual(remaining, []int64{second, open}) {
		t.Errorf("remaining QR accesses = %v, want only the deleted id gone: %v", remaining, []int64{second, open})
	}
}

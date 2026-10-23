package db

import (
	"context"
	"testing"

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

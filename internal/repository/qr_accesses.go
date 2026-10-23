package repository

import (
	"context"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

type QrAccessRepository interface {
	RecordQrAccess(ctx context.Context, access domain.QrAccess) error
}

type UnreportedQrAccessRepository interface {
	ListUnreportedQrAccesses(ctx context.Context, openedBefore time.Time) ([]domain.RecordedQrAccess, error)
	DeleteQrAccesses(ctx context.Context, ids []int64) error
}

package repository

import (
	"context"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
)

type QrAccessRepository interface {
	RecordQrAccess(ctx context.Context, access domain.QrAccess) error
}

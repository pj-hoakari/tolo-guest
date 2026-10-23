package connect

import (
	"context"
	"errors"
	"log/slog"

	connectrpc "connectrpc.com/connect/v2"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	"github.com/pj-hoakari/tolo-guest/internal/tenantctx"
)

// errInternal is the only detail a client learns about an internal failure.
var errInternal = errors.New("internal error")

var clientErrors = []struct {
	target  error
	code    connectrpc.Code
	message string
}{
	{domain.ErrKindRequired, connectrpc.CodeInvalidArgument, "page composition kind is required"},
	{domain.ErrDuplicateKind, connectrpc.CodeInvalidArgument, "page composition kind is duplicated"},
	{domain.ErrComponentUnspecified, connectrpc.CodeInvalidArgument, "page composition component is unspecified"},
	{domain.ErrDuplicateComponent, connectrpc.CodeInvalidArgument, "page composition component is duplicated"},
	{tenantctx.ErrMissing, connectrpc.CodePermissionDenied, "tenant ID is missing from the token"},
	{tenantctx.ErrMismatch, connectrpc.CodePermissionDenied, "tenant ID does not match the token"},
	{tenantctx.ErrEventMissing, connectrpc.CodePermissionDenied, "event ID is missing from the token"},
	{tenantctx.ErrEventMismatch, connectrpc.CodePermissionDenied, "event ID does not match the token"},
}

func connectError(ctx context.Context, err error) *connectrpc.Error {
	for _, candidate := range clientErrors {
		if errors.Is(err, candidate.target) {
			return connectrpc.NewError(candidate.code, candidate.message).WithCause(err)
		}
	}

	return InternalError(ctx, err)
}

// InternalError reports a failure the client can do nothing about. The cause is
// written to the server log and replaced by a fixed message, so that no
// internal detail leaves the service. The log handler names the trace of the
// request context on the record, so an operator can find the failure in the
// trace it belongs to.
//
// A cancelled or timed-out request is the client going away rather than a
// server fault, so it keeps its own code and is not logged.
//
// It is exported for the other transports of this process, so that every
// service answers an internal failure the same way.
func InternalError(ctx context.Context, err error) *connectrpc.Error {
	if errors.Is(err, context.Canceled) {
		return connectrpc.NewError(connectrpc.CodeCanceled, "canceled").WithCause(err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return connectrpc.NewError(connectrpc.CodeDeadlineExceeded, "deadline exceeded").WithCause(err)
	}

	slog.ErrorContext(ctx, "internal error", "error", err)

	return connectrpc.NewError(connectrpc.CodeInternal, "internal error").WithCause(errInternal) //nolint:forbidigo // the one place that builds internal errors
}

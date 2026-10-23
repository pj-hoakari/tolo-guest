package connect

import (
	"context"
	"errors"
	"log/slog"

	connectrpc "connectrpc.com/connect/v2"

	"github.com/pj-hoakari/go-service-template/internal/domain"
)

// errInternal is the only detail a client learns about an internal failure.
var errInternal = errors.New("internal error")

var clientErrors = []struct {
	target  error
	code    connectrpc.Code
	message string
}{
	{domain.ErrGreetingNameRequired, connectrpc.CodeInvalidArgument, "name is required"},
}

func connectError(ctx context.Context, err error) *connectrpc.Error {
	for _, e := range clientErrors {
		if errors.Is(err, e.target) {
			return connectrpc.NewError(e.code, e.message).WithCause(err)
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

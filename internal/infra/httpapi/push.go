package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	observationv1 "github.com/pj-hoakari/tolo-observation/gen/tolo/observation/v1"
	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"google.golang.org/protobuf/proto"

	"github.com/pj-hoakari/tolo-guest/internal/domain"
	connectinfra "github.com/pj-hoakari/tolo-guest/internal/infra/connect"
)

const maxPushBodyBytes = 10 << 20

var errMissingEventID = errors.New("event_id is empty")

type GuestReadModelApplier interface {
	ApplyGuestStatus(ctx context.Context, status domain.GuestStatus) error
	ApplyGuestMessages(ctx context.Context, update domain.GuestMessagesUpdate) error
}

type pushPayload interface {
	proto.Message
	GetEventId() string
}

func PushRoutes(readModel GuestReadModelApplier) Routes {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /push/guest-status", receivePush(
			func() *observationv1.GuestStatus { return new(observationv1.GuestStatus) },
			func(ctx context.Context, status *observationv1.GuestStatus) error {
				return readModel.ApplyGuestStatus(ctx, connectinfra.GuestStatus(status))
			},
		))
		mux.HandleFunc("POST /push/guest-messages", receivePush(
			func() *operationv1.GuestMessagesUpdate { return new(operationv1.GuestMessagesUpdate) },
			func(ctx context.Context, update *operationv1.GuestMessagesUpdate) error {
				return readModel.ApplyGuestMessages(ctx, connectinfra.GuestMessagesUpdate(update))
			},
		))
	}
}

func receivePush[P pushPayload](newPayload func() P, apply func(context.Context, P) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := slog.With(
			"path", r.URL.Path,
			"message_id", r.Header.Get("X-Goog-Pubsub-Message-Id"),
			"ordering_key", r.Header.Get("X-Goog-Pubsub-Ordering-Key"),
			"publish_time", r.Header.Get("X-Goog-Pubsub-Publish-Time"),
		)

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPushBodyBytes))

		var tooLarge *http.MaxBytesError
		if err != nil && !errors.As(err, &tooLarge) {
			logger.ErrorContext(ctx, "read push body failed", "error", err)
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)

			return
		}

		payload := newPayload()
		if err == nil {
			err = proto.Unmarshal(body, payload)
		}

		if err == nil && payload.GetEventId() == "" {
			err = errMissingEventID
		}

		if err != nil {
			logger.ErrorContext(ctx, "discard undecodable push", "error", err)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		if err := apply(ctx, payload); err != nil {
			logger.ErrorContext(ctx, "apply push failed", "event_id", payload.GetEventId(), "error", err)
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)

			return
		}

		logger.DebugContext(ctx, "applied push", "event_id", payload.GetEventId())
		w.WriteHeader(http.StatusNoContent)
	}
}

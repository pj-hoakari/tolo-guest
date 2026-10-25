package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pj-hoakari/tolo-guest/internal/application"
	connectinfra "github.com/pj-hoakari/tolo-guest/internal/infra/connect"
	dbinfra "github.com/pj-hoakari/tolo-guest/internal/infra/db"
	"github.com/pj-hoakari/tolo-guest/internal/infra/httpapi"
	"github.com/pj-hoakari/tolo-guest/internal/logging"
	"github.com/pj-hoakari/tolo-guest/internal/telemetry"
)

const (
	defaultAddr              = ":8080"
	defaultLogLevel          = "info"
	defaultReconcileInterval = 10 * time.Minute
	defaultQrReportInterval  = 30 * time.Second
	upstreamTimeout          = 10 * time.Second
	shutdownTimeout          = 10 * time.Second
	readHeaderTimeout        = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		// run() installs the default logger itself, so a failure before that
		// point is reported by slog's own handler on stderr instead.
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger, err := newLogger()
	if err != nil {
		return err
	}

	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := getenv("SERVER_ADDR", defaultAddr)
	jwtSettings := connectinfra.DefaultJWTSettings()
	jwtSettings.JWKSURL = getenv("INTERNAL_JWKS_URL", jwtSettings.JWKSURL)
	jwtSettings.Issuer = getenv("INTERNAL_JWT_ISSUER", jwtSettings.Issuer)
	jwtSettings.Audience = getenv("INTERNAL_JWT_AUDIENCE", jwtSettings.Audience)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	observationURL := os.Getenv("OBSERVATION_URL")
	if observationURL == "" {
		return errors.New("OBSERVATION_URL is required")
	}

	operationURL := os.Getenv("OPERATION_URL")
	if operationURL == "" {
		return errors.New("OPERATION_URL is required")
	}

	graphAuthoringURL := os.Getenv("GRAPH_AUTHORING_URL")
	if graphAuthoringURL == "" {
		return errors.New("GRAPH_AUTHORING_URL is required")
	}

	reconcileInterval, err := time.ParseDuration(getenv("RECONCILE_INTERVAL", defaultReconcileInterval.String()))
	if err != nil {
		return fmt.Errorf("read RECONCILE_INTERVAL: %w", err)
	}

	if reconcileInterval <= 0 {
		return errors.New("RECONCILE_INTERVAL must be positive")
	}

	qrReportInterval, err := time.ParseDuration(getenv("QR_REPORT_INTERVAL", defaultQrReportInterval.String()))
	if err != nil {
		return fmt.Errorf("read QR_REPORT_INTERVAL: %w", err)
	}

	if qrReportInterval <= 0 {
		return errors.New("QR_REPORT_INTERVAL must be positive")
	}

	shutdownTracing, err := telemetry.Setup(ctx)
	if err != nil {
		return fmt.Errorf("setup tracing: %w", err)
	}
	defer shutdownTracingWithTimeout(shutdownTracing)

	if telemetry.Enabled() {
		slog.Info("tracing enabled", "service", telemetry.ServiceName())
	}

	db, err := dbinfra.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close database failed", "error", err)
		}
	}()

	guestSettings := dbinfra.NewPostgresGuestSettingsRepository(db)
	upstream := &http.Client{Timeout: upstreamTimeout}
	graphAuthoring := connectinfra.NewGraphAuthoringClient(upstream, graphAuthoringURL)
	observation := connectinfra.NewObservationClient(upstream, observationURL)
	qrAccesses := dbinfra.NewPostgresQrAccessRepository(db)
	readModelRepository := dbinfra.NewPostgresGuestReadModelRepository(db)
	readModel := application.NewGuestReadModelService(
		readModelRepository,
		guestSettings,
		observation,
		connectinfra.NewOperationClient(upstream, operationURL),
		graphAuthoring,
		graphAuthoring,
		graphAuthoring,
		qrAccesses,
		application.NewQrOpenLimiter(time.Now),
	)
	qrCounts := application.NewQrCountReporter(qrAccesses, observation)

	backgroundCtx, stopBackground := context.WithCancel(ctx)

	var background sync.WaitGroup

	background.Go(func() { readModel.RunReconciliation(backgroundCtx, reconcileInterval) })
	background.Go(func() { qrCounts.RunQrCountReports(backgroundCtx, qrReportInterval) })

	defer func() {
		stopBackground()
		background.Wait()
	}()

	serviceRoutes, err := connectinfra.RoutesWithJWTSettings(
		jwtSettings,
		application.NewGuestSettingsService(guestSettings, readModelRepository),
		readModel,
	)
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}

	handler := httpapi.NewHandler(
		httpapi.HealthRoutes(httpapi.ReadinessCheck{Name: "database", Check: db.PingContext}),
		httpapi.PushRoutes(readModel),
		serviceRoutes,
	)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		// net/http reports its own failures (a broken connection, a panic in a
		// handler) through this logger, so it goes to the same structured
		// stream as everything else.
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)

	go func() {
		slog.Info("server listening", "addr", addr)

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err

			return
		}

		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		slog.Info("server shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		return httpServer.Shutdown(shutdownCtx)
	}
}

// newLogger builds the process logger from the environment. It is the first
// thing run() does, so that everything the service reports afterwards is
// written in the structure Cloud Logging parses.
func newLogger() (*slog.Logger, error) {
	level, err := logging.ParseLevel(getenv("LOG_LEVEL", defaultLogLevel))
	if err != nil {
		return nil, fmt.Errorf("read LOG_LEVEL: %w", err)
	}

	return logging.NewLogger(os.Stdout, logging.Options{
		Level:     level,
		AddSource: false,
		// Without a project the log entries carry the bare trace ID, so Cloud
		// Logging cannot correlate them with the trace.
		ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT"),
	}), nil
}

// shutdownTracingWithTimeout flushes pending spans on a fresh context, because
// the run context is already cancelled once the process starts shutting down.
func shutdownTracingWithTimeout(shutdown telemetry.ShutdownFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := shutdown(ctx); err != nil {
		slog.Error("shutdown tracing failed", "error", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

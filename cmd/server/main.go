package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/jack5341/otel-map-server/internal/config"
	"github.com/jack5341/otel-map-server/internal/db"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	httpserver "github.com/jack5341/otel-map-server/internal/http"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(errors.Join(errorz.ErrConfigNotFound, err))
	}

	log.Println("config loaded")

	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(otelconfig.WithServiceName(cfg.ServiceName))
	if err != nil {
		exp, expErr := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if expErr != nil {
			panic(errors.Join(errorz.ErrErrorWileStartingOTel, err))
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(sdkresource.NewSchemaless(
				semconv.ServiceName(cfg.ServiceName),
			)),
		)
		shutdownFn := func() { _ = tp.Shutdown(context.Background()) }
		otelShutdown = shutdownFn
	}
	defer otelShutdown()

	database, err := db.Open(cfg.ClickHouseDSN)
	if err != nil {
		panic(errors.Join(errorz.ErrDatabaseError, err))
	}

	log.Println("database initialized")

	e := echo.New()
	otelTracer := otel.Tracer(cfg.ServiceName)
	httpserver.Register(e, database, otelTracer)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	srvErrCh := make(chan error, 1)
	go func() { srvErrCh <- e.StartServer(srv) }()

	log.Println("server initialized")

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-shutdownCtx.Done():
		// graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			_ = e.Close()
		}
	case err := <-srvErrCh:
		if err != nil && err != http.ErrServerClosed {
			// server failed to start or crashed
			fmt.Fprintln(os.Stderr, errors.Join(errorz.ErrServerError, err))
			os.Exit(1)
		}
	}
}

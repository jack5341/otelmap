package http

import (
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"

	"github.com/labstack/echo/v4"

	"github.com/jack5341/otel-map-server/internal/config"
	"github.com/jack5341/otel-map-server/internal/handlers"
	imw "github.com/jack5341/otel-map-server/internal/http/middleware"
)

func Register(e *echo.Echo, db *gorm.DB, otelTracer trace.Tracer, config *config.Config) {
	imw.Apply(e, otelTracer)

	api := e.Group("/api")
	v1 := api.Group("/v1")

	// Handlers
	health := handlers.NewHealthHandler(db, otelTracer)
	serviceMap := handlers.NewServiceMapHandler(db, otelTracer)
	sessionToken := handlers.NewSessionTokenHandler(db, otelTracer, config)
	sessionEvents := handlers.NewSessionEventsHandler(db, otelTracer, config)

	// Health endpoints
	v1.GET("/healthz", health.Liveness)
	v1.GET("/readyz", health.Readiness)

	v1.GET("/service-map/:session-token", serviceMap.Get)
	v1.GET("/session-events", sessionEvents.Listen)
	v1.POST("/session-token", sessionToken.Create)
}

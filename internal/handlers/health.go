package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db         *gorm.DB
	otelTracer trace.Tracer
}

func NewHealthHandler(db *gorm.DB, otelTracer trace.Tracer) *HealthHandler {
	return &HealthHandler{db: db, otelTracer: otelTracer}
}

func (h *HealthHandler) Liveness(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readiness(c echo.Context) error {
	if h.db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db not initialized"})
	}
	sqlDB, err := h.db.DB()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db error"})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db not ready"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

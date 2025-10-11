package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	mapmanager "github.com/jack5341/otel-map-server/pkg/map_manager"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type ServiceMapRequest struct {
	ID uuid.UUID `json:"id"`
}

type ServiceMapHandler struct {
	db         *gorm.DB
	otelTracer trace.Tracer
}

func NewServiceMapHandler(db *gorm.DB, otelTracer trace.Tracer) *ServiceMapHandler {
	return &ServiceMapHandler{db: db, otelTracer: otelTracer}
}

func (h *ServiceMapHandler) Get(c echo.Context) error {
	ctx, span := h.otelTracer.Start(c.Request().Context(), "ServiceMapHandler.Get")
	defer span.End()
	sessionToken := c.Param("session-token")
	if sessionToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errorz.ErrSessionTokenRequired.Error()})
	}

	var tokenUUID, err = uuid.Parse(sessionToken)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errorz.ErrInvalidSessionToken.Error()})
	}

	var token models.SessionToken
	if err := h.db.WithContext(ctx).Find(&token, models.SessionToken{Token: tokenUUID}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errorz.ErrSessionTokenNotFound.Error()})
	}

	if token.Token == uuid.Nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errorz.ErrSessionTokenNotFound.Error()})
	}

	startParam := c.QueryParam("start")
	endParam := c.QueryParam("end")

	var start, end *time.Time
	if startParam != "" {
		if parsed, err := time.Parse(time.RFC3339, startParam); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid start time format"})
		} else {
			start = &parsed
		}
	}
	if endParam != "" {
		if parsed, err := time.Parse(time.RFC3339, endParam); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid end time format"})
		} else {
			end = &parsed
		}
	}

	manager := mapmanager.NewMapManager(h.db, h.otelTracer)
	dto, err := manager.Create(tokenUUID, start, end, ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, dto)
}

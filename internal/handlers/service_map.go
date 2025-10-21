package handlers

import (
	"net/http"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	mapz "github.com/jack5341/otel-map-server/internal/mapz"
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

type ServiceMapResponse struct {
	Services []mapz.Service `json:"services"`
	Edges    []mapz.Edge    `json:"edges"`
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

	var _, err = uuid.Parse(sessionToken)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errorz.ErrInvalidSessionToken.Error()})
	}

	mapper := mapz.NewMapper(h.db, h.otelTracer, ctx)
	services, err := mapper.GetServicesWithMetrics(sessionToken)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	edges, err := mapper.GetEdges(sessionToken)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	serviceMapResponse := ServiceMapResponse{
		Services: services,
		Edges:    edges,
	}

	return c.JSON(http.StatusOK, serviceMapResponse)
}

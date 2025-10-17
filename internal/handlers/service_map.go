package handlers

import (
	"net/http"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	pkg "github.com/jack5341/otel-map-server/pkg/mapper"
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

	var spans []models.OtelTrace

	if err := h.db.WithContext(ctx).Where("ResourceAttributes['otelmap.session_token'] = ?", tokenUUID.String()).Find(&spans).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	mapper := pkg.NewMapper(h.otelTracer, ctx)
	otelSpans := make([]pkg.Span, len(spans))
	for i, s := range spans {
		otelSpans[i] = pkg.Span{
			TraceId:            s.TraceId,
			SpanId:             s.SpanId,
			ParentSpanId:       s.ParentSpanId,
			ServiceName:        s.ServiceName,
			SpanName:           s.SpanName,
			SpanKind:           s.SpanKind,
			Timestamp:          s.Timestamp,
			Duration:           s.Duration,
			StatusCode:         s.StatusCode,
			SpanAttributes:     s.SpanAttributes,
			ResourceAttributes: s.ResourceAttributes,
			Events:             s.Events,
			Links:              s.Links,
		}
	}
	nodes, err := mapper.Create(spans)
	return c.JSON(http.StatusOK, nodes)
}

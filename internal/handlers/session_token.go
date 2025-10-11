package handlers

import (
	"net/http"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type SessionTokenHandler struct {
	db         *gorm.DB
	otelTracer trace.Tracer
}

func NewSessionTokenHandler(db *gorm.DB, otelTracer trace.Tracer) *SessionTokenHandler {
	return &SessionTokenHandler{db: db, otelTracer: otelTracer}
}

func (h *SessionTokenHandler) Create(c echo.Context) error {
	token := uuid.New()
	ctx, span := h.otelTracer.Start(c.Request().Context(), "SessionTokenHandler.Create")
	defer span.End()
	if err := h.db.WithContext(ctx).Create(&models.SessionToken{Token: token}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errorz.ErrWhileCreatingSessionToken.Error()})
	}

	resp := map[string]any{
		"token": token.String(),
		"ingest": map[string]any{
			"otlp_http_url": "https://collector.example.com/v1/traces",
			"header_key":    "X-OTEL-SESSION",
			"header_value":  token.String(),
			"resource_attribute": map[string]any{
				"key":   "otelmap.session_token",
				"value": token.String(),
			},
		},
	}
	return c.JSON(http.StatusOK, resp)
}

package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jack5341/otel-map-server/internal/config"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type SessionTokenHandler struct {
	db         *gorm.DB
	otelTracer trace.Tracer
	config     *config.Config
}

type IngestConfig struct {
	OTLPHTTPURL       string `json:"otlp_http_url"`
	OTLPGRPCURL       string `json:"otlp_grpc_url"`
	HeaderKey         string `json:"header_key"`
	HeaderValue       string `json:"header_value"`
	ResourceAttribute struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"resource_attribute"`
}

type SessionTokenResponse struct {
	Token  string       `json:"token"`
	Ingest IngestConfig `json:"ingest"`
}

func NewSessionTokenHandler(db *gorm.DB, otelTracer trace.Tracer, config *config.Config) *SessionTokenHandler {
	return &SessionTokenHandler{db: db, otelTracer: otelTracer, config: config}
}

func (h *SessionTokenHandler) Create(c echo.Context) error {
	token := uuid.New()
	ctx, span := h.otelTracer.Start(c.Request().Context(), "SessionTokenHandler.Create")
	defer span.End()
	if err := h.db.WithContext(ctx).Create(&models.SessionToken{Token: token}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errorz.ErrWhileCreatingSessionToken.Error()})
	}

	resp := SessionTokenResponse{
		Token: token.String(),
		Ingest: IngestConfig{
			OTLPHTTPURL: "https://otlp." + h.config.BaseURL + "/v1/traces",
			OTLPGRPCURL: "https://otlp." + h.config.BaseURL + "/opentelemetry.proto",
			HeaderKey:   "X-OTEL-SESSION",
			HeaderValue: token.String(),
			ResourceAttribute: struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}{
				Key:   "otelmap.session_token",
				Value: token.String(),
			},
		},
	}
	return c.JSON(http.StatusOK, resp)
}

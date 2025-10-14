package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jack5341/otel-map-server/internal/config"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type SessionEventsHandler struct {
	db         *gorm.DB
	otelTracer trace.Tracer
	config     *config.Config
}

func NewSessionEventsHandler(db *gorm.DB, otelTracer trace.Tracer, config *config.Config) *SessionEventsHandler {
	return &SessionEventsHandler{db: db, otelTracer: otelTracer, config: config}
}

func (h *SessionEventsHandler) Listen(c echo.Context) error {
	ctx, span := h.otelTracer.Start(c.Request().Context(), "SessionEventsHandler.Listen")
	defer span.End()

	// If a token query param is provided, switch to SSE polling mode
	tokenParam := c.QueryParam("token")
	if tokenParam != "" {
		tokenUUID, err := uuid.Parse(tokenParam)
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

		// Prepare SSE headers
		res := c.Response()
		res.Header().Set(echo.HeaderContentType, "text/event-stream")
		res.Header().Set(echo.HeaderCacheControl, "no-cache")
		res.Header().Set(echo.HeaderConnection, "keep-alive")
		res.Header().Set("X-Accel-Buffering", "no")
		res.WriteHeader(http.StatusOK)

		flusher, ok := res.Writer.(http.Flusher)
		if !ok {
			return c.NoContent(http.StatusInternalServerError)
		}

		_, _ = res.Write([]byte(": open\n\n"))
		flusher.Flush()

		hasTraces := func() (bool, error) {
			var count int64
			if err := h.db.WithContext(ctx).Raw(
				"SELECT count() FROM otel.otel_traces WHERE SpanAttributes['otelmap.session_token'] = ? LIMIT 1",
				tokenUUID.String(),
			).Scan(&count).Error; err != nil {
				return false, err
			}
			return count > 0, nil
		}

		sentEvent := false
		if okFound, err := hasTraces(); err == nil && okFound {
			_, _ = res.Write([]byte("event: traces_received\ndata: {}\n\n"))
			flusher.Flush()
			sentEvent = true
		}

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-c.Request().Context().Done():
				return nil
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if !sentEvent {
					found, err := hasTraces()
					if err != nil {
						return nil
					}
					if found {
						_, _ = res.Write([]byte("event: traces_received\ndata: {}\n\n"))
						flusher.Flush()
						sentEvent = true
					}
				}
				_, _ = res.Write([]byte(": keepalive\n\n"))
				flusher.Flush()
			}
		}
	}
	return nil
}

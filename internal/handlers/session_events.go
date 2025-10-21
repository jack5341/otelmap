package handlers

import (
	"encoding/json"
	"fmt"
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

type SessionEventResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

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
			if err := h.db.WithContext(ctx).
				Model(&models.OtelTrace{}).
				Raw("SELECT count() FROM default.otel_traces WHERE ResourceAttributes['otelmap.session_token'] = ?", tokenUUID.String()).
				Scan(&count).Error; err != nil {
				return false, err
			}
			return count > 0, nil
		}

		sentEvent := false
		if okFound, err := hasTraces(); err == nil && okFound {
			eventData, _ := json.Marshal(SessionEventResponse{Status: "received"})
			_, _ = res.Write([]byte(fmt.Sprintf("event: traces_received\ndata: %s\n\n", eventData)))
			flusher.Flush()
			sentEvent = true
		}

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		timeout := time.NewTimer(60 * time.Second)
		defer timeout.Stop()

		for {
			select {
			case <-c.Request().Context().Done():
				return nil
			case <-ctx.Done():
				return nil
			case <-timeout.C:
				return nil
			case <-ticker.C:
				if !sentEvent {
					found, err := hasTraces()
					if err != nil {
						_ = err // ignore error
						return nil
					}
					if found {
						eventData, _ := json.Marshal(SessionEventResponse{Status: "received"})
						_, _ = res.Write([]byte(fmt.Sprintf("event: traces_received\ndata: %s\n\n", eventData)))
						flusher.Flush()
						sentEvent = true
					} else {
						eventData, _ := json.Marshal(SessionEventResponse{Status: "waiting"})
						_, _ = res.Write([]byte(fmt.Sprintf("event: waiting_trace\ndata: %s\n\n", eventData)))
						flusher.Flush()
					}
				} else {
					_, _ = res.Write([]byte(": keepalive\n\n"))
					flusher.Flush()
				}
			}
		}
	}
	return nil
}

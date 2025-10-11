package middleware

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/otel/trace"
)

func Apply(e *echo.Echo, otelTracer trace.Tracer) {
	e.HideBanner = true
	e.HidePort = true

	e.Use(echomw.Recover())
	e.Use(echomw.RequestID())
	e.Use(echomw.Logger())
	e.Use(echomw.Secure())
	e.Use(echomw.CORS())
	e.Use(echomw.RateLimiter(echomw.NewRateLimiterMemoryStore(20)))
}

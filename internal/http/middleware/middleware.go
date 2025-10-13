package middleware

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			propagators := otel.GetTextMapPropagator()
			ctx := propagators.Extract(c.Request().Context(), propagation.HeaderCarrier(c.Request().Header))
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
}

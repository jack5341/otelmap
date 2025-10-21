package mapz

import (
	"context"
	"errors"

	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type Mapper struct {
	db         *gorm.DB
	otelTracer trace.Tracer
	ctx        context.Context
}

const getEdgesQuery = `
WITH parameters AS (
    SELECT
        ? AS session_token
),
Duration AS (
    SELECT 
        (toUnixTimestamp(MAX(Timestamp)) - toUnixTimestamp(MIN(Timestamp))) AS window_seconds
    FROM default.otel_traces, parameters
    WHERE ResourceAttributes['otelmap.session_token'] = parameters.session_token
    HAVING window_seconds >= 1
),

SpansBase AS (
    SELECT
        t.TraceId,
        t.SpanId,
        t.ParentSpanId,
        t.ServiceName AS ServiceName, 
        
        multiIf(
            has(t.SpanAttributes, 'http.route'),
            t.SpanAttributes['http.method'] || ' ' || t.SpanAttributes['http.route'],
            t.SpanName
        ) AS Path
    FROM default.otel_traces AS t, parameters
    WHERE t.ResourceAttributes['otelmap.session_token'] = parameters.session_token
),

ServiceNode AS (
    SELECT
        TraceId,
        SpanId,
        ParentSpanId,
        ServiceName,
        Path
    FROM SpansBase
    GROUP BY TraceId, SpanId, ParentSpanId, ServiceName, Path
)

SELECT
    p.ServiceName AS source_service_name, 
    c.ServiceName AS target_service_name,
    c.Path AS target_service_path,
    COUNT() AS total_requests,
    ROUND(COUNT() / (SELECT window_seconds FROM Duration), 2) AS requests_per_second
FROM ServiceNode AS c
INNER JOIN ServiceNode AS p
    ON c.ParentSpanId = p.SpanId 
    AND c.TraceId = p.TraceId
WHERE c.ParentSpanId != '' AND c.ParentSpanId IS NOT NULL 
GROUP BY source_service_name, target_service_name, target_service_path
ORDER BY source_service_name, target_service_name, target_service_path
`

const getServicesWithMetricsQuery = `
SELECT
    t.ServiceName AS service_name,
    COUNT() AS total_requests,
    SUM(multiIf(t.StatusCode = '2', 1, 0)) AS error_count,
    ROUND(error_count / total_requests, 4) AS error_rate,
    ROUND(quantileTDigest(0.50)(t.Duration) / 1000000, 2) AS latency_p50_ms,
    ROUND(quantileTDigest(0.90)(t.Duration) / 1000000, 2) AS latency_p90_ms,
    ROUND(quantileTDigest(0.95)(t.Duration) / 1000000, 2) AS latency_p95_ms
FROM otel_traces AS t
WHERE t.ResourceAttributes['otelmap.session_token'] = ?
GROUP BY service_name
ORDER BY total_requests DESC
`

type Edge struct {
	SourceServiceName string  `json:"source_service_name"`
	TargetServiceName string  `json:"target_service_name"`
	TargetServicePath string  `json:"target_service_path"`
	TotalRequests     uint64  `json:"total_requests"`
	RequestsPerSecond float64 `json:"requests_per_second"`
}
type Service struct {
	ServiceName   string  `json:"service_name"`
	TotalRequests int64   `json:"total_requests"`
	ErrorCount    int64   `json:"error_count"`
	ErrorRate     float64 `json:"error_rate"`
	LatencyP50Ms  float64 `json:"latency_p50_ms"`
	LatencyP90Ms  float64 `json:"latency_p90_ms"`
	LatencyP95Ms  float64 `json:"latency_p95_ms"`
}

func NewMapper(db *gorm.DB, otelTracer trace.Tracer, ctx context.Context) *Mapper {
	return &Mapper{db: db, otelTracer: otelTracer, ctx: ctx}
}

func (m *Mapper) GetEdges(sessionToken string) ([]Edge, error) {
	ctx, span := m.otelTracer.Start(m.ctx, "Mapper.GetEdges")
	defer span.End()

	if sessionToken == "" {
		return nil, errorz.ErrSessionTokenRequired
	}

	var edges []Edge
	err := m.db.WithContext(ctx).Raw(getEdgesQuery, sessionToken).Scan(&edges).Error
	if err != nil {
		return nil, errors.Join(errorz.ErrWhileGettingEdges, err)
	}

	return edges, nil
}

func (m *Mapper) GetServicesWithMetrics(sessionToken string) ([]Service, error) {
	ctx, span := m.otelTracer.Start(m.ctx, "Mapper.GetServicesWithMetrics")
	defer span.End()

	if sessionToken == "" {
		return nil, errorz.ErrSessionTokenRequired
	}

	var services []Service
	err := m.db.WithContext(ctx).Raw(getServicesWithMetricsQuery, sessionToken).Scan(&services).Error
	if err != nil {
		return nil, errors.Join(errorz.ErrWhileGettingServicesWithMetrics, err)
	}

	return services, nil
}

package mapmanager

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type MapDTO struct {
	RequestFlows  []RequestFlow `json:"request_flows"`
	Services      []Service     `json:"services"`
	GlobalMetrics GlobalMetrics `json:"global_metrics"`
}

type MapManager struct {
	db         *gorm.DB
	otelTracer trace.Tracer
}

type Service struct {
	Name          string  `json:"name"`
	Count         int     `json:"count"`
	Rps           float64 `json:"rps"`
	ErrorRate     float64 `json:"error_rate"`
	ThroughputBps float64 `json:"throughput_bps"`
}

type RequestFlow struct {
	Service Service       `json:"service"`
	Childs  []RequestFlow `json:"childs"`
}

type GlobalMetrics struct {
	TotalServices int     `json:"total_services"`
	TotalRequests int     `json:"total_requests"`
	AvgRps        float64 `json:"avg_rps"`
	ErrorRate     float64 `json:"error_rate"`
}

func NewMapManager(db *gorm.DB, otelTracer trace.Tracer) *MapManager {
	return &MapManager{db: db, otelTracer: otelTracer}
}

func (m *MapManager) Create(token uuid.UUID, start *time.Time, end *time.Time, ctx context.Context) (MapDTO, error) {
	ctx, span := m.otelTracer.Start(ctx, "MapManager.Create")
	defer span.End()
	var rows []models.OtelTrace
	query := `SELECT * FROM default.otel_traces WHERE ResourceAttributes['otelmap.session_token'] = ?`
	args := []interface{}{token.String()}

	if start != nil {
		query += ` AND Timestamp >= ?`
		args = append(args, *start)
	}
	if end != nil {
		query += ` AND Timestamp <= ?`
		args = append(args, *end)
	}

	if err := m.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		errWrapped := errors.Join(errorz.ErrWhileGettingOtelTraces, err)
		span.RecordError(errWrapped)
		span.SetStatus(codes.Error, errWrapped.Error())
		return MapDTO{}, errWrapped
	}

	requestFlows := m.buildRequestFlows(rows)
	globalMetrics, services, err := m.buildGlobalMetricsAndServices(ctx, rows)
	if err != nil {
		errWrapped := errors.Join(errorz.ErrWhileBuildingGlobalMetricsAndServices, err)
		span.RecordError(errWrapped)
		span.SetStatus(codes.Error, errWrapped.Error())
		return MapDTO{}, errWrapped
	}

	return MapDTO{
		RequestFlows:  requestFlows,
		Services:      services,
		GlobalMetrics: globalMetrics,
	}, nil
}

func (m *MapManager) buildGlobalMetricsAndServices(ctx context.Context, rows []models.OtelTrace) (GlobalMetrics, []Service, error) {
	ctx, span := m.otelTracer.Start(ctx, "MapManager.buildGlobalMetricsAndServices")
	defer span.End()

	var (
		totalRequests int64
		totalErrors   int64
		services      []Service
	)

	var serviceNames = make(map[string]bool)
	for _, r := range rows {
		if _, ok := serviceNames[r.ServiceName]; !ok {
			var (
				traceCount int64
				errorCount int64
			)

			oneMinuteAgo := time.Now().Add(-1 * time.Minute)
			err := m.db.WithContext(ctx).
				Model(&models.OtelTrace{}).
				Where("ServiceName = ?", r.ServiceName).
				Where("Timestamp >= ?", oneMinuteAgo).
				Count(&traceCount).Error

			if err != nil {
				errWrapped := errors.Join(errorz.ErrWhileGettingOtelTraceCount, err)
				span.RecordError(errWrapped)
				span.SetStatus(codes.Error, errWrapped.Error())
				return GlobalMetrics{}, nil, errWrapped
			}

			err = m.db.WithContext(ctx).
				Model(&models.OtelTrace{}).
				Where("ServiceName = ?", r.ServiceName).
				Where("StatusCode = ?", "Error").
				Where("Timestamp >= ?", oneMinuteAgo).
				Count(&errorCount).Error

			if err != nil {
				errWrapped := errors.Join(errorz.ErrWhileGettingOtelTraceErrorCount, err)
				span.RecordError(errWrapped)
				span.SetStatus(codes.Error, errWrapped.Error())
				return GlobalMetrics{}, nil, errWrapped
			}

			throughputBps := float64(traceCount) * float64(r.Duration) / 1000000

			var errorRate float64
			if traceCount > 0 {
				errorRate = float64(errorCount) / float64(traceCount)
			} else {
				errorRate = 0
			}

			services = append(services, Service{
				Name:          r.ServiceName,
				Count:         int(traceCount),
				Rps:           float64(traceCount) / time.Since(oneMinuteAgo).Seconds(),
				ThroughputBps: throughputBps,
				ErrorRate:     errorRate,
			})

			serviceNames[r.ServiceName] = true
		}
	}

	oneMinuteAgo := time.Now().Add(-1 * time.Minute)
	err := m.db.WithContext(ctx).
		Model(&models.OtelTrace{}).
		Where("Timestamp >= ?", oneMinuteAgo).
		Count(&totalRequests).Error

	if err != nil {
		errWrapped := errors.Join(errorz.ErrWhileGettingOtelTraceCount, err)
		span.RecordError(errWrapped)
		span.SetStatus(codes.Error, errWrapped.Error())
		return GlobalMetrics{}, nil, errWrapped
	}

	err = m.db.WithContext(ctx).
		Model(&models.OtelTrace{}).
		Where("StatusCode = ?", "Error").
		Where("Timestamp >= ?", oneMinuteAgo).
		Count(&totalErrors).Error

	if err != nil {
		errWrapped := errors.Join(errorz.ErrWhileGettingOtelTraceErrorCount, err)
		span.RecordError(errWrapped)
		span.SetStatus(codes.Error, errWrapped.Error())
		return GlobalMetrics{}, nil, errWrapped
	}

	var globalErrorRate float64
	if totalRequests > 0 {
		globalErrorRate = float64(totalErrors) / float64(totalRequests)
	} else {
		globalErrorRate = 0
	}

	return GlobalMetrics{
		TotalServices: len(services),
		TotalRequests: int(totalRequests),
		AvgRps:        float64(totalRequests) / time.Since(oneMinuteAgo).Seconds(),
		ErrorRate:     globalErrorRate,
	}, services, nil
}

func (m *MapManager) buildRequestFlows(rows []models.OtelTrace) []RequestFlow {
	spanIdToRow := make(map[string]models.OtelTrace, len(rows))
	childrenByParent := make(map[string][]models.OtelTrace, len(rows))
	for _, r := range rows {
		spanIdToRow[r.SpanId] = r
		if r.ParentSpanId != "" {
			childrenByParent[r.ParentSpanId] = append(childrenByParent[r.ParentSpanId], r)
		}
	}

	var roots []models.OtelTrace
	for _, r := range rows {
		if r.ParentSpanId == "" {
			roots = append(roots, r)
			continue
		}
		if _, ok := spanIdToRow[r.ParentSpanId]; !ok {
			roots = append(roots, r)
		}
	}

	var build func(models.OtelTrace) RequestFlow
	build = func(r models.OtelTrace) RequestFlow {
		var childFlows []RequestFlow
		if childs, ok := childrenByParent[r.SpanId]; ok {
			childFlows = make([]RequestFlow, 0, len(childs))
			for _, c := range childs {
				childFlows = append(childFlows, build(c))
			}
		}
		return RequestFlow{
			Service: Service{
				Name: r.ServiceName,
			},
			Childs: childFlows,
		}
	}

	requestFlows := make([]RequestFlow, 0, len(roots))
	for _, root := range roots {
		requestFlows = append(requestFlows, build(root))
	}
	return requestFlows
}

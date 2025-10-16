package mapmanager

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
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
		return MapDTO{}, errors.Join(errorz.ErrWhileGettingOtelTraces, err)
	}

	requestFlows := m.buildRequestFlows(rows)
	globalMetrics, services := m.buildGlobalMetricsAndServices(ctx, rows)

	return MapDTO{
		RequestFlows:  requestFlows,
		Services:      services,
		GlobalMetrics: globalMetrics,
	}, nil
}

func (m *MapManager) buildGlobalMetricsAndServices(ctx context.Context, rows []models.OtelTrace) (GlobalMetrics, []Service) {
	var (
		totalRequests int64
		totalErrors   int64
		services      []Service
	)

	var serviceNames = make(map[string]bool)
	for _, r := range rows {
		if _, ok := serviceNames[r.ServiceName]; !ok {
			var (
				count  int64
				errors int64
			)

			oneMinuteAgo := time.Now().Add(-1 * time.Minute)
			m.db.WithContext(ctx).
				Model(&models.OtelTrace{}).
				Where("service_name = ?", r.ServiceName).
				Where("timestamp >= ?", oneMinuteAgo).
				Count(&count)

			m.db.WithContext(ctx).
				Model(&models.OtelTrace{}).
				Where("service_name = ?", r.ServiceName).
				Where("status_code = 'Error'").
				Where("timestamp >= ?", oneMinuteAgo).
				Count(&errors)

			throughputBps := float64(count) * float64(r.Duration) / 1000000

			services = append(services, Service{
				Name:          r.ServiceName,
				Count:         int(count),
				Rps:           float64(count) / time.Since(oneMinuteAgo).Seconds(),
				ThroughputBps: throughputBps,
				ErrorRate:     float64(errors) / float64(count),
			})

			serviceNames[r.ServiceName] = true
		}
	}

	oneMinuteAgo := time.Now().Add(-1 * time.Minute)
	m.db.WithContext(ctx).
		Model(&models.OtelTrace{}).
		Where("timestamp >= ?", oneMinuteAgo).
		Count(&totalRequests)
	m.db.WithContext(ctx).
		Model(&models.OtelTrace{}).
		Where("status_code = 'Error'").
		Where("timestamp >= ?", oneMinuteAgo).
		Count(&totalErrors)

	return GlobalMetrics{
		TotalServices: len(services),
		TotalRequests: int(totalRequests),
		AvgRps:        float64(totalRequests) / time.Since(oneMinuteAgo).Seconds(),
		ErrorRate:     float64(totalErrors) / float64(totalRequests),
	}, services
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

	// Determine roots: no parent or parent not included in this dataset
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

	// Recursive constructor for RequestFlow
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

package mapmanager

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"github.com/jack5341/otel-map-server/internal/models"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Node struct {
	Service      string   `json:"service"`
	RPS          float64  `json:"rps"`
	LatencyMsAvg int64    `json:"latency_ms_avg"`
	P50Ms        int64    `json:"p50_ms"`
	P95Ms        int64    `json:"p95_ms"`
	P99Ms        int64    `json:"p99_ms"`
	ErrorRate    float64  `json:"error_rate"`
	Position     Position `json:"position"`
}

type Edge struct {
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	Requests     int64   `json:"requests"`
	LatencyMsAvg int64   `json:"latency_ms_avg"`
	ErrorRate    float64 `json:"error_rate"`
}

type MapDTO struct {
	Global map[string]any `json:"global"`
	Nodes  []Node         `json:"nodes"`
	Edges  []Edge         `json:"edges"`
}

type MapManager struct {
	db         *gorm.DB
	otelTracer trace.Tracer
}

type serviceStats struct {
	requests    int64
	latencySum  int64
	fiveXXCount int64
}
type edgeKey struct{ src, dst string }
type edgeStats struct {
	requests   int64
	latencySum int64
	errors     int64
}

func NewMapManager(db *gorm.DB, otelTracer trace.Tracer) *MapManager {
	return &MapManager{db: db, otelTracer: otelTracer}
}

func (m *MapManager) Create(token uuid.UUID, start *time.Time, end *time.Time, ctx context.Context) (MapDTO, error) {
	ctx, span := m.otelTracer.Start(context.Background(), "MapManager.Create")
	defer span.End()
	var rows []models.OtelTrace
	query := `SELECT * FROM otel.otel_traces WHERE SpanAttributes['otelmap.session_token'] = ?`
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

	spanService := make(map[string]string)
	services := make(map[string]*serviceStats)
	serviceDurations := make(map[string][]int64)
	edges := make(map[edgeKey]*edgeStats)
	var totalRequests int64

	for _, r := range rows {
		spanService[r.TraceId+"|"+r.SpanId] = r.ServiceName
	}

	for _, r := range rows {
		serviceName := r.ServiceName
		if serviceName == "" {
			continue
		}

		spanKindUp := strings.ToUpper(r.SpanKind)
		isServer := spanKindUp == "SERVER"
		durationMs := r.Duration / 1_000_000

		statusCodeStr, ok := r.SpanAttributes["http.status_code"]
		statusCodeInt := 0
		if ok {
			if v, err := strconv.Atoi(statusCodeStr); err == nil {
				statusCodeInt = v
			}
		}
		is5xx := statusCodeInt >= 500 && statusCodeInt < 600

		ss, ok2 := services[serviceName]
		if !ok2 {
			ss = &serviceStats{}
			services[serviceName] = ss
		}

		if isServer {
			ss.requests++
			ss.latencySum += durationMs
			if is5xx {
				ss.fiveXXCount++
			}
			totalRequests++
			serviceDurations[serviceName] = append(serviceDurations[serviceName], durationMs)
		}

		if r.ParentSpanId != "" {
			parentSvc := spanService[r.TraceId+"|"+r.ParentSpanId]
			if parentSvc != "" && parentSvc != serviceName {
				key := edgeKey{src: parentSvc, dst: serviceName}
				es, ok := edges[key]
				if !ok {
					es = &edgeStats{}
					edges[key] = es
				}
				if isServer {
					es.requests++
					es.latencySum += durationMs
					if is5xx {
						es.errors++
					}
				}
			}
		}
	}

	windowStart := time.Now().UTC()
	var windowEnd time.Time
	for _, r := range rows {
		if r.Timestamp.Before(windowStart) {
			windowStart = r.Timestamp
		}
		if r.Timestamp.After(windowEnd) {
			windowEnd = r.Timestamp
		}
	}
	if windowEnd.Before(windowStart) {
		windowEnd = windowStart.Add(time.Second)
	}
	windowSeconds := windowEnd.Sub(windowStart).Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}

	global := map[string]any{
		"total_services": len(services),
		"total_requests": totalRequests,
		"avg_rps":        float64(totalRequests) / windowSeconds,
		"time_range": map[string]string{
			"from": windowStart.Format(time.RFC3339),
			"to":   windowEnd.Format(time.RFC3339),
		},
	}

	percentile := func(durs []int64, p float64) int64 {
		if len(durs) == 0 {
			return 0
		}
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		pos := int(math.Ceil(p*float64(len(durs)))) - 1
		if pos < 0 {
			pos = 0
		}
		if pos >= len(durs) {
			pos = len(durs) - 1
		}
		return durs[pos]
	}

	nodes := make([]Node, 0, len(services))
	i := 0
	for name, stats := range services {
		avgLatency := int64(0)
		if stats.requests > 0 {
			avgLatency = stats.latencySum / stats.requests
		}
		p50, p95, p99 := int64(0), int64(0), int64(0)
		if durs, ok := serviceDurations[name]; ok && len(durs) > 0 {

			cpy := make([]int64, len(durs))
			copy(cpy, durs)
			p50 = percentile(cpy, 0.50)
			copy(cpy, durs)
			p95 = percentile(cpy, 0.95)
			copy(cpy, durs)
			p99 = percentile(cpy, 0.99)
		}
		rate5xx := 0.0
		if stats.requests > 0 {
			rate5xx = float64(stats.fiveXXCount) / float64(stats.requests)
		}

		posX := (i%6)*180 + 120
		posY := (i/6)*170 + 140
		nodes = append(nodes, Node{
			Service:      name,
			RPS:          float64(stats.requests) / windowSeconds,
			LatencyMsAvg: avgLatency,
			P50Ms:        p50,
			P95Ms:        p95,
			P99Ms:        p99,
			ErrorRate:    rate5xx,
			Position:     Position{X: posX, Y: posY},
		})
		i++
	}

	mapEdges := make([]Edge, 0, len(edges))
	for key, es := range edges {
		avgLatency := int64(0)
		if es.requests > 0 {
			avgLatency = es.latencySum / es.requests
		}
		errRate := 0.0
		if es.requests > 0 {
			errRate = float64(es.errors) / float64(es.requests)
		}
		mapEdges = append(mapEdges, Edge{
			Source:       key.src,
			Target:       key.dst,
			Requests:     es.requests,
			LatencyMsAvg: avgLatency,
			ErrorRate:    errRate,
		})
	}

	return MapDTO{Global: global, Nodes: nodes, Edges: mapEdges}, nil
}

package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	errorz "github.com/jack5341/otel-map-server/internal/errors"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type Mapper struct {
	otelTracer trace.Tracer
	ctx        context.Context
	dbConfig   DBConfig
}

type DBConfig struct {
	Db           *gorm.DB
	TableName    string
	SessionToken string
	SessionKey   string
}

type OtelTrace struct {
	TraceId            string            `gorm:"primaryKey;type:String" json:"trace_id"`
	SpanId             string            `gorm:"type:String" json:"span_id"`
	ParentSpanId       string            `gorm:"type:String" json:"parent_span_id"`
	ServiceName        string            `gorm:"index;type:String" json:"ServiceName"`
	SpanName           string            `gorm:"type:String" json:"span_name"`
	SpanKind           string            `gorm:"type:String" json:"span_kind"`
	Timestamp          time.Time         `gorm:"type:DateTime" json:"timestamp"`
	Duration           int64             `gorm:"type:UInt64" json:"duration"`
	StatusCode         string            `gorm:"type:String" json:"status_code"`
	SpanAttributes     map[string]string `gorm:"type:Map(String,String)" json:"span_attributes"`
	ResourceAttributes map[string]string `gorm:"type:Map(String,String)" json:"resource_attributes"`
	Events             json.RawMessage   `gorm:"type:String" json:"events"`
	Links              json.RawMessage   `gorm:"type:String" json:"links"`
}

type ServiceMap struct {
	Services []Service `json:"services"`
	Edges    []Edge    `json:"edges"`
}

type Edge struct {
	Source *Service `json:"source"`
	Rps    float64  `json:"rps"`
	Target *Service `json:"target"`
}

type Service struct {
	Name          string  `json:"name"`
	Count         int     `json:"count"`
	Rps           float64 `json:"rps"`
	ThroughputBps float64 `json:"throughput_bps"`
	ErrorRate     float64 `json:"error_rate"`
}

func NewMapper(dbConfig DBConfig, options ...interface{}) *Mapper {
	var otelTracer trace.Tracer
	var ctx context.Context

	for _, opt := range options {
		switch v := opt.(type) {
		case trace.Tracer:
			otelTracer = v
		case context.Context:
			ctx = v
		case DBConfig:
			dbConfig = v
		}
	}

	return &Mapper{otelTracer: otelTracer, ctx: ctx, dbConfig: dbConfig}
}

func (m *Mapper) Create() (ServiceMap, error) {
	ctx, span := m.otelTracer.Start(m.ctx, "Mapper.Create")
	defer span.End()

	var rows []OtelTrace
	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE ResourceAttributes['%s'] = ?",
		m.dbConfig.TableName,
		m.dbConfig.SessionKey,
	)
	if err := m.dbConfig.Db.WithContext(ctx).Raw(query, m.dbConfig.SessionToken).Scan(&rows).Error; err != nil {
		errWrapped := errors.Join(errorz.ErrWhileGettingOtelTraces, err)
		span.RecordError(errWrapped)
		span.SetStatus(codes.Error, errWrapped.Error())
		return ServiceMap{}, errWrapped
	}

	serviceNameToService := buildServices(rows)
	edges := buildEdges(rows, serviceNameToService)
	services := make([]Service, 0, len(serviceNameToService))
	for _, s := range serviceNameToService {
		services = append(services, *s)
	}
	return ServiceMap{Services: services, Edges: edges}, nil
}

func buildServices(rows []OtelTrace) map[string]*Service {
	services := make(map[string]*Service)

	oneMinuteAgo := time.Now().Add(-1 * time.Minute)
	secondsSince := time.Since(oneMinuteAgo).Seconds()
	if secondsSince <= 0 {
		secondsSince = 60
	}

	type agg struct {
		totalCount           int
		lastMinuteCount      int
		lastMinuteErrorCount int
		lastMinuteDurNS      int64
	}
	serviceAgg := make(map[string]*agg)

	for _, r := range rows {
		a, ok := serviceAgg[r.ServiceName]
		if !ok {
			a = &agg{}
			serviceAgg[r.ServiceName] = a
		}
		a.totalCount++
		if !r.Timestamp.Before(oneMinuteAgo) {
			a.lastMinuteCount++
			a.lastMinuteDurNS += r.Duration
			if r.StatusCode == "Error" {
				a.lastMinuteErrorCount++
			}
		}
	}

	for name, a := range serviceAgg {
		var rps float64
		if a.lastMinuteCount > 0 {
			rps = float64(a.lastMinuteCount) / secondsSince
		}
		var errRate float64
		if a.lastMinuteCount > 0 {
			errRate = float64(a.lastMinuteErrorCount) / float64(a.lastMinuteCount)
		}
		var throughputBps float64
		if a.lastMinuteDurNS > 0 {
			throughputBps = float64(a.lastMinuteDurNS) / secondsSince
		}

		services[name] = &Service{
			Name:          name,
			Count:         a.totalCount,
			Rps:           rps,
			ThroughputBps: throughputBps,
			ErrorRate:     errRate,
		}
	}

	return services
}

func buildEdges(rows []OtelTrace, serviceNameToService map[string]*Service) []Edge {
	spanByID := make(map[string]OtelTrace, len(rows))
	for _, r := range rows {
		spanByID[r.SpanId] = r
	}

	oneMinuteAgo := time.Now().Add(-1 * time.Minute)
	secondsSince := time.Since(oneMinuteAgo).Seconds()
	if secondsSince <= 0 {
		secondsSince = 60
	}

	seen := make(map[string]bool)
	counts := make(map[string]int)
	var edges []Edge

	for _, child := range rows {
		if child.ParentSpanId == "" {
			continue
		}
		parent, ok := spanByID[child.ParentSpanId]
		if !ok {
			continue
		}
		if parent.ServiceName == child.ServiceName {
			continue
		}

		source := serviceNameToService[parent.ServiceName]
		target := serviceNameToService[child.ServiceName]
		if source == nil || target == nil {
			continue
		}

		key := parent.ServiceName + "->" + child.ServiceName
		if !child.Timestamp.Before(oneMinuteAgo) {
			counts[key]++
		}
		if !seen[key] {
			seen[key] = true
			edges = append(edges, Edge{Source: source, Target: target})
		}
	}

	// Assign RPS to edges
	for i := range edges {
		key := edges[i].Source.Name + "->" + edges[i].Target.Name
		if c, ok := counts[key]; ok && c > 0 {
			edges[i].Rps = float64(c) / secondsSince
		} else {
			edges[i].Rps = 0
		}
	}

	return edges
}

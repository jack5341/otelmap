package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	errorz "github.com/jack5341/otel-map-server/internal/errors"
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
	Source Service `json:"source"`
	Target string  `json:"target"`
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
	_, span := m.otelTracer.Start(m.ctx, "Mapper.Create")
	defer span.End()

	var serviceMap ServiceMap
	serviceNames, err := m.getServiceNames(m.ctx)
	if err != nil {
		return ServiceMap{}, err
	}

	for _, name := range serviceNames {
		service, err := m.buildServiceMetrics(m.ctx, OtelTrace{ServiceName: name})
		if err != nil {
			return ServiceMap{}, err
		}
		serviceMap.Services = append(serviceMap.Services, service)
	}

	return serviceMap, nil
}

func (m *Mapper) getSpans(ctx context.Context) ([]OtelTrace, error) {
	var spans []OtelTrace
	if err := m.dbConfig.Db.WithContext(ctx).Where(fmt.Sprintf("ResourceAttributes['%s'] = ?", m.dbConfig.SessionKey), m.dbConfig.SessionToken).Find(&spans).Error; err != nil {
		return nil, errors.Join(errorz.ErrWhileGettingOtelTraces, err)
	}

	return spans, nil
}

func (m *Mapper) getServiceNames(ctx context.Context) ([]string, error) {
	var names []string
	query := m.dbConfig.Db.WithContext(ctx).
		Table(m.dbConfig.TableName).
		Select("DISTINCT ServiceName").
		Where(fmt.Sprintf("ResourceAttributes['%s'] = ?", m.dbConfig.SessionKey), m.dbConfig.SessionToken)

	if err := query.Pluck("ServiceName", &names).Error; err != nil {
		return nil, errors.Join(errorz.ErrWhileGettingOtelTraces, err)
	}

	return names, nil
}

func (m *Mapper) buildServiceMetrics(ctx context.Context, span OtelTrace) (Service, error) {
	var traceCount int64
	err := m.dbConfig.Db.WithContext(ctx).
		Table(m.dbConfig.TableName).
		Where("ServiceName = ?", span.ServiceName).
		Where(fmt.Sprintf("ResourceAttributes['%s'] = ?", m.dbConfig.SessionKey), m.dbConfig.SessionToken).
		Count(&traceCount).Error
	if err != nil {
		return Service{}, errors.Join(errorz.ErrWhileGettingOtelTraceCount, err)
	}

	var errorCount int64
	err = m.dbConfig.Db.WithContext(ctx).
		Table(m.dbConfig.TableName).
		Where("ServiceName = ?", span.ServiceName).
		Where(fmt.Sprintf("ResourceAttributes['%s'] = ?", m.dbConfig.SessionKey), m.dbConfig.SessionToken).
		Where("StatusCode = ?", "Error").
		Count(&errorCount).Error
	if err != nil {
		return Service{}, errors.Join(errorz.ErrWhileGettingOtelTraceErrorRate, err)
	}

	var errorRate float64
	if traceCount > 0 {
		errorRate = float64(errorCount) / float64(traceCount)
	} else {
		errorRate = 0
	}

	return Service{
		Name:      span.ServiceName,
		Count:     int(traceCount),
		ErrorRate: errorRate,
	}, nil
}

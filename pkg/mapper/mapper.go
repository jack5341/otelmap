package pkg

import (
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type Mapper struct {
	otelTracer trace.Tracer
	ctx        context.Context
	spans      []Span
}

type RequestFlow struct {
	TraceId string `json:"trace_id"`
	Spans   []Span `json:"spans"`
}

type Span struct {
	TraceId            string
	SpanId             string
	ParentSpanId       string
	ServiceName        string
	SpanName           string
	SpanKind           string
	Timestamp          time.Time
	Duration           int64
	StatusCode         string
	SpanAttributes     map[string]string
	ResourceAttributes map[string]string
	Events             json.RawMessage
	Links              json.RawMessage
}

func NewMapper(options ...interface{}) *Mapper {
	var otelTracer trace.Tracer
	var ctx context.Context

	for _, opt := range options {
		switch v := opt.(type) {
		case trace.Tracer:
			otelTracer = v
		case context.Context:
			ctx = v
		}
	}

	return &Mapper{otelTracer: otelTracer, ctx: ctx}
}

func (m *Mapper) Create(spans []Span) ([]Node, error) {
	_, span := m.otelTracer.Start(m.ctx, "Mapper.Create")
	defer span.End()
	m.spans = spans
	nodes := m.buildGraph()
	return nodes, nil
}

type Node struct {
	SpanId   string
	Children []Node
}

func (m *Mapper) buildGraph() []Node {
	return []Node{}
}

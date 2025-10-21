package models

import (
	"encoding/json"
	"time"
)

type OtelTrace struct {
	TraceId            string            `gorm:"primaryKey;type:String" json:"trace_id"`
	SpanId             string            `gorm:"type:String" json:"span_id"`
	ParentSpanId       string            `gorm:"type:String" json:"parent_span_id"`
	ServiceName        string            `gorm:"index;type:String" json:"service_name"`
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

// Use fully qualified database.table for ClickHouse OTEL dataset
func (OtelTrace) TableName() string { return "default.otel_traces" }

package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ServiceMap struct {
	ID           uuid.UUID       `gorm:"type:uuid;primary_key;"`
	Fingerprint  string          `gorm:"type:string;uniqueIndex"`
	SessionToken string          `gorm:"type:string"`
	MapData      json.RawMessage `gorm:"type:string"`
	CreatedAt    time.Time
}

func (ServiceMap) TableName() string {
	return "session_tokens.service_maps"
}

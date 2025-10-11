package models

import (
	"time"

	"github.com/google/uuid"
)

type SessionToken struct {
	Token     uuid.UUID `gorm:"primaryKey;type:UUID" json:"token"`
	CreatedAt time.Time `gorm:"type:DateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:DateTime" json:"updated_at"`
}

func (SessionToken) TableName() string { return "session_tokens" }

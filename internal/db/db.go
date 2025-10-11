package db

import (
	"context"
	"time"

	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/jack5341/otel-map-server/internal/models"
)

func Open(dsn string) (*gorm.DB, error) {
	gormDB, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetConnMaxLifetime(time.Minute * 5)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)

	// Ensure connection is alive at startup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, err
	}

	if err := gormDB.Use(tracing.NewPlugin()); err != nil {
		panic(err)
	}

	// Skip OtelTrace auto-migration as it conflicts with ClickHouse schema
	err = gormDB.AutoMigrate(&models.SessionToken{})
	if err != nil {
		return nil, err
	}

	return gormDB, nil
}

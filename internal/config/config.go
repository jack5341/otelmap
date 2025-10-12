package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Port             string        `env:"PORT" envDefault:"8000"`
	ClickHouseDSN    string        `env:"CLICKHOUSE_DSN" envDefault:"clickhouse://default:default@localhost:9000/default?dial_timeout=5s&compress=true"`
	LogLevel         string        `env:"LOG_LEVEL" envDefault:"info"`
	ServiceName      string        `env:"SERVICE_NAME" envDefault:"default"`
	ShutdownTimeoutS int           `env:"SHUTDOWN_TIMEOUT_SECONDS" envDefault:"10"`
	ShutdownTimeout  time.Duration `env:"-"`
}

func Load() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	cfg.ShutdownTimeout = time.Duration(cfg.ShutdownTimeoutS) * time.Second
	return cfg, nil
}

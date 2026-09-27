package main

import (
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

type Config struct {
	HttpAddr        string        `env:"HTTP_ADDR,notEmpty"`
	LogLevel        string        `env:"LOG_LEVEL,notEmpty"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT,notEmpty" validate:"gt=0"`

	HTTPReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT,notEmpty" validate:"gt=0"`
	HTTPReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT,notEmpty" validate:"gt=0"`
	HTTPWriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT,notEmpty" validate:"gt=0"`
	HTTPIdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT,notEmpty" validate:"gt=0"`

	DatabaseUrl             string        `env:"DATABASE_URL,notEmpty"`
	DatabaseMaxConns        int           `env:"DATABASE_MAX_CONNS,notEmpty"`
	DatabaseMinConns        int           `env:"DATABASE_MIN_CONNS,notEmpty"`
	DatabaseMaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME,notEmpty" validate:"gt=0"`
	DatabaseConnectTimeout  time.Duration `env:"DATABASE_CONNECT_TIMEOUT,notEmpty" validate:"gt=0"`
	DatabaseQueryTimeout    time.Duration `env:"DATABASE_QUERY_TIMEOUT,notEmpty" validate:"gt=0"`
}

func NewConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

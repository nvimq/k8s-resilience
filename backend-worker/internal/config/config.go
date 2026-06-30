package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	GRPC    GRPCConfig
	DB      DBConfig
	Redis   RedisConfig
	OTel    OTelConfig
	Version string
}

type GRPCConfig struct {
	Port            int           `envconfig:"GRPC_PORT" default:"50051"`
	ShutdownTimeout time.Duration `envconfig:"GRPC_SHUTDOWN_TIMEOUT" default:"15s"`
}

type DBConfig struct {
	URL             string        `envconfig:"DATABASE_URL" required:"true"`
	MaxConns        int           `envconfig:"DB_MAX_CONNS" default:"50"`
	MinConns        int           `envconfig:"DB_MIN_CONNS" default:"5"`
	MaxConnLifetime time.Duration `envconfig:"DB_MAX_CONN_LIFETIME" default:"30m"`
	MaxConnIdleTime time.Duration `envconfig:"DB_MAX_CONN_IDLE_TIME" default:"5m"`
	ConnTimeout     time.Duration `envconfig:"DB_CONN_TIMEOUT" default:"2s"`
	QueryTimeout    time.Duration `envconfig:"DB_QUERY_TIMEOUT" default:"1s"`
}

type RedisConfig struct {
	Addr     string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
	Password string `envconfig:"REDIS_PASSWORD"`
	DB       int    `envconfig:"REDIS_DB" default:"0"`
}

type OTelConfig struct {
	OTLPExportEndpoint string `envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://otel-collector:4318"`
	ServiceName        string `envconfig:"OTEL_SERVICE_NAME" default:"backend-worker"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

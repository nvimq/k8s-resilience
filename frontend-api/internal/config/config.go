package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	HTTP    HTTPConfig
	GRPC    GRPCClientConfig
	OTel    OTelConfig
	Version string
}

type HTTPConfig struct {
	Port            int           `envconfig:"HTTP_PORT" default:"8080"`
	ShutdownTimeout time.Duration `envconfig:"HTTP_SHUTDOWN_TIMEOUT" default:"15s"`
	ReadTimeout     time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"10s"`
	WriteTimeout    time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"10s"`
}

type GRPCClientConfig struct {
	BackendAddr string        `envconfig:"GRPC_BACKEND_ADDR" default:"localhost:50051"`
	Timeout     time.Duration `envconfig:"GRPC_TIMEOUT" default:"5s"`
}

type OTelConfig struct {
	OTLPExportEndpoint string `envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://otel-collector:4318"`
	ServiceName        string `envconfig:"OTEL_SERVICE_NAME" default:"frontend-api"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Port                     string  `mapstructure:"PORT"`
	PostgresHost             string  `mapstructure:"POSTGRES_HOST"`
	PostgresPort             string  `mapstructure:"POSTGRES_PORT"`
	PostgresUser             string  `mapstructure:"POSTGRES_USER"`
	PostgresPassword         string  `mapstructure:"POSTGRES_PASSWORD"`
	PostgresDB               string  `mapstructure:"POSTGRES_DB"`
	RedisHost                string  `mapstructure:"REDIS_HOST"`
	RedisPort                string  `mapstructure:"REDIS_PORT"`
	OTELExporterOTLPEndpoint        string  `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELExporterOTLPMetricsEndpoint string  `mapstructure:"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"`
	OTELServiceName                 string  `mapstructure:"OTEL_SERVICE_NAME"`
	OTELServiceVersion              string  `mapstructure:"OTEL_SERVICE_VERSION"`
	OTELServiceEnvironment          string  `mapstructure:"OTEL_SERVICE_ENVIRONMENT"`
	OTELTraceSampleRatio            float64 `mapstructure:"OTEL_TRACE_SAMPLE_RATIO"`
	OTELMetricsExportIntervalSec    int     `mapstructure:"OTEL_METRICS_EXPORT_INTERVAL_SEC"`
}

func Load(path string) (*Config, error) {
	viper.AddConfigPath(path)
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("config: reading file: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Port == "" {
		c.Port = "5000"
	}
	if c.OTELServiceName == "" {
		c.OTELServiceName = "user-service"
	}
	if c.OTELServiceVersion == "" {
		c.OTELServiceVersion = "0.1.0"
	}
	if c.OTELServiceEnvironment == "" {
		c.OTELServiceEnvironment = "development"
	}
	if c.OTELExporterOTLPEndpoint == "" {
		c.OTELExporterOTLPEndpoint = "localhost:4317"
	}
	if c.OTELExporterOTLPMetricsEndpoint == "" {
		c.OTELExporterOTLPMetricsEndpoint = "localhost:9090"
	}
	if c.OTELTraceSampleRatio == 0 {
		c.OTELTraceSampleRatio = 1.0
	}
	if c.OTELMetricsExportIntervalSec == 0 {
		c.OTELMetricsExportIntervalSec = 15
	}
}

func (c *Config) validate() error {
	required := map[string]string{
		"POSTGRES_HOST":     c.PostgresHost,
		"POSTGRES_PORT":     c.PostgresPort,
		"POSTGRES_USER":     c.PostgresUser,
		"POSTGRES_PASSWORD": c.PostgresPassword,
		"POSTGRES_DB":       c.PostgresDB,
		"REDIS_HOST":        c.RedisHost,
		"REDIS_PORT":        c.RedisPort,
	}

	for key, val := range required {
		if val == "" {
			return fmt.Errorf("config: %s is required", key)
		}
	}

	return nil
}

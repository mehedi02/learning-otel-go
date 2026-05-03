package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/mehedi/user-service-go/internal/config"
)

func NewRedis(cfg *config.Config, log *slog.Logger) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort)

	client := redis.NewClient(&redis.Options{Addr: addr})

	// Install OTel tracing hook on the client BEFORE the first command so
	// even the startup Ping below produces a span.
	if err := redisotel.InstrumentTracing(client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: instrument tracing: %w", err)
	}

	// Pool/command metrics via the global MeterProvider.
	if err := redisotel.InstrumentMetrics(client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: instrument metrics: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	log.Info("connected to redis", "addr", addr)

	return client, nil
}

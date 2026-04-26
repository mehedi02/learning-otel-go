package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mehedi/user-service-go/internal/config"
	"github.com/redis/go-redis/v9"
)

func NewRedis(cfg *config.Config, log *slog.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	log.Info("connected to redis", "addr", fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort))

	return client, nil
}
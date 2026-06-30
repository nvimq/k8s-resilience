package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/resume/k8s-resilience/backend-worker/internal/config"
	"github.com/resume/k8s-resilience/backend-worker/internal/model"
)

const taskCacheTTL = 30 * time.Second

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(cfg config.RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	slog.Info("connected to redis", "addr", cfg.Addr)
	return &RedisCache{client: client}, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

func (c *RedisCache) GetTask(ctx context.Context, id string) (*model.Task, error) {
	data, err := c.client.Get(ctx, cacheKey(id)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("redis get %s: %w", id, err)
	}

	task := &model.Task{}
	if err := json.Unmarshal(data, task); err != nil {
		return nil, fmt.Errorf("unmarshal task %s: %w", id, err)
	}

	return task, nil
}

func (c *RedisCache) SetTask(ctx context.Context, task *model.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task %s: %w", task.ID, err)
	}

	if err := c.client.Set(ctx, cacheKey(task.ID), data, taskCacheTTL).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", task.ID, err)
	}

	return nil
}

func (c *RedisCache) HealthCheck(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func cacheKey(id string) string {
	return fmt.Sprintf("task:%s", id)
}

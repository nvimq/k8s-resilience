package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/resume/k8s-resilience/backend-worker/internal/model"
)

var (
	ErrNotFound      = errors.New("task not found")
	ErrDBUnavailable = errors.New("database unavailable")
)

type Store struct {
	pg    *PostgresStore
	redis *RedisCache
}

func NewStore(pg *PostgresStore, redis *RedisCache) *Store {
	return &Store{pg: pg, redis: redis}
}

func (s *Store) GetTask(ctx context.Context, id string) (*model.Task, string, error) {
	task, err := s.pg.GetTask(ctx, id)
	if err == nil {
		if cacheErr := s.redis.SetTask(ctx, task); cacheErr != nil {
			slog.Warn("failed to update redis cache", "task_id", id, "error", cacheErr)
		}
		return task, "PostgreSQL", nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}

	slog.Warn("postgres get failed, falling back to redis", "task_id", id, "error", err)

	task, cacheErr := s.redis.GetTask(ctx, id)
	if cacheErr != nil {
		return nil, "", errors.Join(ErrDBUnavailable, err, cacheErr)
	}

	slog.Info("served from redis cache", "task_id", id)
	return task, "Redis_Cache", nil
}

func (s *Store) CreateTask(ctx context.Context, id, title, description string) (*model.Task, string, error) {
	task, err := s.pg.CreateTask(ctx, id, title, description)
	if err == nil {
		if cacheErr := s.redis.SetTask(ctx, task); cacheErr != nil {
			slog.Warn("failed to update redis cache on create", "task_id", id, "error", cacheErr)
		}
		return task, "PostgreSQL", nil
	}

	slog.Warn("postgres create failed, storing in redis only", "error", err)

	fallbackTask := &model.Task{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      "created_offline",
	}

	if cacheErr := s.redis.SetTask(ctx, fallbackTask); cacheErr != nil {
		return nil, "", errors.Join(ErrDBUnavailable, err, cacheErr)
	}

	slog.Info("task stored in redis fallback", "task_id", id)
	return fallbackTask, "Redis_Cache", nil
}

func (s *Store) HealthCheck(ctx context.Context) error {
	return s.pg.HealthCheck(ctx)
}

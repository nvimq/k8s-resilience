package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"github.com/resume/k8s-resilience/backend-worker/internal/config"
	"github.com/resume/k8s-resilience/backend-worker/internal/model"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, cfg config.DBConfig) (*PostgresStore, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	slog.Info("connected to postgresql", "max_conns", cfg.MaxConns)
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) GetTask(ctx context.Context, id string) (*model.Task, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	task := &model.Task{}
	err := s.pool.QueryRow(queryCtx,
		`SELECT id, title, description, status, created_at, updated_at FROM tasks WHERE id = $1`, id,
	).Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("query task %s: %w", id, err)
	}

	return task, nil
}

func (s *PostgresStore) CreateTask(ctx context.Context, id, title, description string) (*model.Task, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	task := &model.Task{}
	err := s.pool.QueryRow(queryCtx,
		`INSERT INTO tasks (id, title, description, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'created', NOW(), NOW())
		 RETURNING id, title, description, status, created_at, updated_at`,
		id, title, description,
	).Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}

func (s *PostgresStore) HealthCheck(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return s.pool.Ping(queryCtx)
}

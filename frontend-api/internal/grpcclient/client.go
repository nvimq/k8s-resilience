package grpcclient

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	workerv1 "github.com/resume/k8s-resilience/api/proto/worker/v1"
	"github.com/resume/k8s-resilience/frontend-api/internal/config"
)

type Client struct {
	client  workerv1.TaskServiceClient
	breaker *gobreaker.CircuitBreaker
	conn    *grpc.ClientConn
}

func NewClient(cfg config.GRPCClientConfig) (*Client, error) {
	retryOpts := []retry.CallOption{
		retry.WithMax(3),
		retry.WithBackoff(retry.BackoffExponentialWithJitter(100*time.Millisecond, 2.0)),
		retry.WithCodes(codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted),
	}

	conn, err := grpc.NewClient(cfg.BackendAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor(retryOpts...)),
	)
	if err != nil {
		return nil, fmt.Errorf("create gRPC connection: %w", err)
	}

	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "backend-worker-grpc",
		MaxRequests: 1,
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.4
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	slog.Info("gRPC client initialized",
		"backend", cfg.BackendAddr,
	)

	return &Client{
		client:  workerv1.NewTaskServiceClient(conn),
		breaker: breaker,
		conn:    conn,
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) GetTask(ctx context.Context, id string) (*workerv1.TaskResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	type result struct {
		resp *workerv1.TaskResponse
		err  error
	}

	ch := make(chan result, 1)

	go func() {
		resp, err := c.breaker.Execute(func() (any, error) {
			return c.client.GetTaskData(ctx, &workerv1.TaskRequest{Id: id})
		})
		if err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{resp: resp.(*workerv1.TaskResponse)}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return c.fallbackResponse(id, r.err), nil
		}
		r.resp.DataSource = appendFallbackSuffix(r.resp.DataSource)
		return r.resp, nil

	case <-ctx.Done():
		return c.fallbackResponse(id, ctx.Err()), nil
	}
}

func (c *Client) CreateTask(ctx context.Context, title, description string) (*workerv1.TaskResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	type result struct {
		resp *workerv1.TaskResponse
		err  error
	}

	ch := make(chan result, 1)

	go func() {
		resp, err := c.breaker.Execute(func() (any, error) {
			return c.client.CreateTask(ctx, &workerv1.CreateTaskRequest{
				Title:       title,
				Description: description,
			})
		})
		if err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{resp: resp.(*workerv1.TaskResponse)}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return &workerv1.TaskResponse{
				Id:          "fallback",
				Title:       title,
				Description: description,
				Status:      "pending_offline",
				ServedByPod: "frontend-circuit-breaker",
				DataSource:  "Circuit_Breaker_Fallback",
			}, nil
		}
		r.resp.DataSource = appendFallbackSuffix(r.resp.DataSource)
		return r.resp, nil

	case <-ctx.Done():
		return &workerv1.TaskResponse{
			Id:          "fallback",
			Title:       title,
			Description: description,
			Status:      "pending_offline",
			ServedByPod: "frontend-circuit-breaker",
			DataSource:  "Circuit_Breaker_Fallback",
		}, nil
	}
}

func (c *Client) HealthCheck(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := c.client.GetTaskData(pingCtx, &workerv1.TaskRequest{Id: "health-check"})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("gRPC health check failed: %w", err)
	}
	return nil
}

func (c *Client) fallbackResponse(id string, err error) *workerv1.TaskResponse {
	slog.Warn("circuit breaker open or request failed, returning fallback",
		"task_id", id, "error", err,
	)

	return &workerv1.TaskResponse{
		Id:          id,
		Title:       "unavailable",
		Description: "Service temporarily unavailable",
		Status:      "unknown",
		ServedByPod: "frontend-circuit-breaker",
		DataSource:  "Circuit_Breaker_Fallback",
	}
}

func appendFallbackSuffix(source string) string {
	if source == "Redis_Cache" {
		return "Redis_Cache"
	}
	return source
}

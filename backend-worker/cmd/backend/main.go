package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	workerv1 "github.com/resume/k8s-resilience/api/proto/worker/v1"
	"github.com/resume/k8s-resilience/backend-worker/internal/config"
	"github.com/resume/k8s-resilience/backend-worker/internal/db"
	"github.com/resume/k8s-resilience/backend-worker/internal/server"
	"github.com/resume/k8s-resilience/backend-worker/internal/telemetry"
	"github.com/resume/k8s-resilience/backend-worker/internal/version"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting backend-worker", "version", version.Version, "commit", version.GitCommit)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tp, err := telemetry.InitTracer(ctx, cfg.OTel.OTLPExportEndpoint, cfg.OTel.ServiceName)
	if err != nil {
		slog.Warn("failed to init tracer, continuing without", "error", err)
	} else {
		defer func() {
			_ = tp.Shutdown(context.Background())
		}()
	}

	pgStore, err := db.NewPostgresStore(ctx, cfg.DB)
	if err != nil {
		slog.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pgStore.Close()

	redisCache, err := db.NewRedisCache(cfg.Redis)
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisCache.Close()

	store := db.NewStore(pgStore, redisCache)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPC.Port))
	if err != nil {
		slog.Error("failed to listen", "port", cfg.GRPC.Port, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)

	taskServer := server.NewTaskServer(store)
	workerv1.RegisterTaskServiceServer(grpcServer, taskServer)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("worker.TaskService", grpc_health_v1.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)

	go func() {
		slog.Info("gRPC server listening", "port", cfg.GRPC.Port)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutting down", "signal", sig.String())

	gracefulCtx, gracefulCancel := context.WithTimeout(context.Background(), cfg.GRPC.ShutdownTimeout)
	defer gracefulCancel()

	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		healthServer.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("graceful shutdown complete")
	case <-gracefulCtx.Done():
		slog.Warn("shutdown timeout exceeded, forcing stop")
		grpcServer.Stop()
	}
}

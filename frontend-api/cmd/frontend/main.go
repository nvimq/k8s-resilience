package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/resume/k8s-resilience/frontend-api/internal/config"
	"github.com/resume/k8s-resilience/frontend-api/internal/grpcclient"
	"github.com/resume/k8s-resilience/frontend-api/internal/handlers"
	"github.com/resume/k8s-resilience/frontend-api/internal/telemetry"
	"github.com/resume/k8s-resilience/frontend-api/internal/version"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting frontend-api", "version", version.Version, "commit", version.GitCommit)

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

	grpcClient, err := grpcclient.NewClient(cfg.GRPC)
	if err != nil {
		slog.Error("failed to create gRPC client", "error", err)
		os.Exit(1)
	}
	defer grpcClient.Close()

	taskHandler := handlers.NewTaskHandler(grpcClient)
	healthHandler := handlers.NewHealthHandler(grpcClient)

	mux := http.NewServeMux()

	mux.Handle("GET /api/v1/tasks/{id}", otelhttp.NewHandler(
		http.HandlerFunc(taskHandler.GetTask),
		"get_task",
	))
	mux.Handle("POST /api/v1/tasks", otelhttp.NewHandler(
		http.HandlerFunc(taskHandler.CreateTask),
		"create_task",
	))

	mux.Handle("GET /healthz", otelhttp.NewHandler(
		http.HandlerFunc(healthHandler.Liveness),
		"liveness",
	))
	mux.Handle("GET /readyz", otelhttp.NewHandler(
		http.HandlerFunc(healthHandler.Readiness),
		"readiness",
	))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      mux,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		slog.Info("HTTP server listening", "port", cfg.HTTP.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutting down", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
}

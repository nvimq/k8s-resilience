package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workerv1 "github.com/resume/k8s-resilience/api/proto/worker/v1"
	"github.com/resume/k8s-resilience/backend-worker/internal/db"
)

var tracer = otel.Tracer("backend-worker")

type TaskServer struct {
	workerv1.UnimplementedTaskServiceServer
	store *db.Store
}

func NewTaskServer(store *db.Store) *TaskServer {
	return &TaskServer{store: store}
}

func (s *TaskServer) GetTaskData(ctx context.Context, req *workerv1.TaskRequest) (*workerv1.TaskResponse, error) {
	ctx, span := tracer.Start(ctx, "TaskService.GetTaskData", trace.WithAttributes(
		attribute.String("task.id", req.Id),
	))
	defer span.End()

	task, dataSource, err := s.store.GetTask(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		return nil, status.Errorf(codes.NotFound, "task %s: %v", req.Id, err)
	}

	return &workerv1.TaskResponse{
		Id:          task.ID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		ServedByPod: os.Getenv("HOSTNAME"),
		DataSource:  dataSource,
	}, nil
}

func (s *TaskServer) CreateTask(ctx context.Context, req *workerv1.CreateTaskRequest) (*workerv1.TaskResponse, error) {
	ctx, span := tracer.Start(ctx, "TaskService.CreateTask", trace.WithAttributes(
		attribute.String("task.title", req.Title),
	))
	defer span.End()

	id := fmt.Sprintf("task-%d", time.Now().UnixNano())
	task, dataSource, err := s.store.CreateTask(ctx, id, req.Title, req.Description)
	if err != nil {
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "create task: %v", err)
	}

	slog.Info("task created",
		"id", task.ID,
		"title", task.Title,
		"data_source", dataSource,
	)

	return &workerv1.TaskResponse{
		Id:          task.ID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		ServedByPod: os.Getenv("HOSTNAME"),
		DataSource:  dataSource,
	}, nil
}

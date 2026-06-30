package handlers

import (
	"encoding/json"
	"net/http"

	"log/slog"

	"github.com/resume/k8s-resilience/frontend-api/internal/grpcclient"
	"github.com/resume/k8s-resilience/frontend-api/internal/model"
)

type TaskHandler struct {
	client *grpcclient.Client
}

func NewTaskHandler(client *grpcclient.Client) *TaskHandler {
	return &TaskHandler{client: client}
}

func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{
			Error:   "invalid_request",
			Message: "task ID is required",
		})
		return
	}

	taskResp, err := h.client.GetTask(r.Context(), id)
	if err != nil {
		slog.Error("failed to get task", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
		return
	}

	task := model.Task{
		ID:          taskResp.Id,
		Title:       taskResp.Title,
		Description: taskResp.Description,
		Status:      taskResp.Status,
		ServedByPod: taskResp.ServedByPod,
		DataSource:  taskResp.DataSource,
	}

	writeJSON(w, http.StatusOK, task)
}

func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{
			Error:   "invalid_body",
			Message: "invalid JSON body",
		})
		return
	}

	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{
			Error:   "validation_error",
			Message: "title is required",
		})
		return
	}

	taskResp, err := h.client.CreateTask(r.Context(), req.Title, req.Description)
	if err != nil {
		slog.Error("failed to create task", "title", req.Title, "error", err)
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
		return
	}

	task := model.Task{
		ID:          taskResp.Id,
		Title:       taskResp.Title,
		Description: taskResp.Description,
		Status:      taskResp.Status,
		ServedByPod: taskResp.ServedByPod,
		DataSource:  taskResp.DataSource,
	}

	writeJSON(w, http.StatusCreated, task)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

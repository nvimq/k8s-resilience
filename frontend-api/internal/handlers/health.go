package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"log/slog"
)

type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

type HealthHandler struct {
	backend HealthChecker
	mu      sync.RWMutex
	healthy bool
}

func NewHealthHandler(backend HealthChecker) *HealthHandler {
	h := &HealthHandler{
		backend: backend,
		healthy: true,
	}
	go h.backgroundCheck()
	return h
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	healthy := h.healthy
	h.mu.RUnlock()

	if !healthy {
		slog.Warn("readiness check failed")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (h *HealthHandler) backgroundCheck() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := h.backend.HealthCheck(ctx)
		cancel()

		h.mu.Lock()
		if err != nil {
			h.healthy = false
			slog.Warn("backend health check failed", "error", err)
		} else {
			h.healthy = true
		}
		h.mu.Unlock()
	}
}

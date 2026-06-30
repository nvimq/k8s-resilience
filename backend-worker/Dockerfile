FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY backend-worker/go.mod backend-worker/go.sum ./backend-worker/
COPY api/go.mod api/go.sum ./api/

RUN cd backend-worker && go mod download

COPY api/ ./api/
COPY backend-worker/ ./backend-worker/

RUN cd backend-worker && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags="-X github.com/resume/k8s-resilience/backend-worker/internal/version.Version=$(git describe --tags --always 2>/dev/null || echo dev) -X github.com/resume/k8s-resilience/backend-worker/internal/version.GitCommit=$(git rev-parse HEAD 2>/dev/null || echo unknown)" \
	-o /app/bin/backend \
	./cmd/backend

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/backend /backend

USER 65532:65532

EXPOSE 50051

ENTRYPOINT ["/backend"]

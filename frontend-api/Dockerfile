FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY internal/version/ ./internal/version/

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags="-X github.com/resume/k8s-resilience/frontend-api/internal/version.Version=$(git describe --tags --always 2>/dev/null || echo dev) -X github.com/resume/k8s-resilience/frontend-api/internal/version.GitCommit=$(git rev-parse HEAD 2>/dev/null || echo unknown)" \
	-o /app/bin/frontend \
	./cmd/frontend

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/frontend /frontend

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/frontend"]

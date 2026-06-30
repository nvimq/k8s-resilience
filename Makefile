.PHONY: help dev proto lint test build build-images kind-up kind-down \
	deploy-infra deploy-apps deploy-observability deploy-chaos \
	attack-network attack-pods full-demo clean

SHELL := /bin/bash
GO ?= go
DOCKER ?= docker
KUBECTL ?= kubectl
HELM ?= helm

# ─── Help ─────────────────────────────────────────────────────────────────────
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'

# ─── Development ──────────────────────────────────────────────────────────────
dev: ## docker-compose up (local dev)
	@docker compose -f deployments/docker-compose/docker-compose.yaml up --build -d

dev-down: ## docker-compose down
	@docker compose -f deployments/docker-compose/docker-compose.yaml down -v

proto: ## Generate protobuf code
	cd hack && bash gen-proto.sh

lint: ## Run golangci-lint on all modules
	cd frontend-api && $(GO) vet ./...
	cd backend-worker && $(GO) vet ./...
	golangci-lint run ./frontend-api/... ./backend-worker/...

test: ## Run tests with race detector
	cd frontend-api && $(GO) test -race -count=1 -coverprofile=coverage.out ./...
	cd backend-worker && $(GO) test -race -count=1 -coverprofile=coverage.out ./...

build: ## Build all binaries
	cd frontend-api && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build \
		-ldflags="-X github.com/resume/k8s-resilience/frontend-api/internal/version.Version=$(shell git describe --tags --always 2>/dev/null || echo dev) \
		-X github.com/resume/k8s-resilience/frontend-api/internal/version.GitCommit=$(shell git rev-parse HEAD 2>/dev/null || echo unknown)" \
		-o ../bin/frontend-api ./cmd/frontend
	cd backend-worker && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build \
		-ldflags="-X github.com/resume/k8s-resilience/backend-worker/internal/version.Version=$(shell git describe --tags --always 2>/dev/null || echo dev) \
		-X github.com/resume/k8s-resilience/backend-worker/internal/version.GitCommit=$(shell git rev-parse HEAD 2>/dev/null || echo unknown)" \
		-o ../bin/backend-worker ./cmd/backend

build-images: ## Build Docker images
	$(DOCKER) build -t frontend-api:latest -f frontend-api/Dockerfile frontend-api/
	$(DOCKER) build -t backend-worker:latest -f backend-worker/Dockerfile backend-worker/

# ─── KIND ─────────────────────────────────────────────────────────────────────
kind-up: ## Create kind cluster
	kind create cluster --config deployments/kind/kind-config.yaml --name k8s-resilience
	$(KUBECTL) create namespace resilience

kind-down: ## Delete kind cluster
	kind delete cluster --name k8s-resilience

kind-load: ## Load images into kind
	kind load docker-image frontend-api:latest --name k8s-resilience
	kind load docker-image backend-worker:latest --name k8s-resilience

# ─── Deploy ────────────────────────────────────────────────────────────────────
deploy-infra: ## Deploy PostgreSQL and Redis via Helm
	$(HELM) repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null; true
	$(HELM) upgrade --install postgres bitnami/postgresql \
		--namespace resilience \
		--set auth.username=app \
		--set auth.password=appsecret \
		--set auth.database=tasks \
		--set primary.persistence.size=1Gi \
		--wait
	$(HELM) upgrade --install redis bitnami/redis \
		--namespace resilience \
		--set architecture=standalone \
		--set auth.password=redissecret \
		--set master.persistence.enabled=true \
		--set master.persistence.size=1Gi \
		--wait

deploy-apps: kind-load ## Deploy microservices Helm chart
	$(HELM) upgrade --install microservices deployments/helm/microservices \
		--namespace resilience \
		--wait

deploy-observability: ## Deploy OTel Collector + Prometheus + Grafana
	$(HELM) repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts 2>/dev/null; true
	$(HELM) repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null; true
	$(HELM) upgrade --install otel-collector open-telemetry/opentelemetry-collector \
		--namespace resilience \
		-f observability/otel-collector-config.yaml \
		--wait
	$(HELM) upgrade --install kube-prometheus prometheus-community/kube-prometheus-stack \
		--namespace resilience \
		--set grafana.dashboardsConfigMaps.resilience-dashboard={resilience-grafana-dashboard} \
		--wait

deploy-chaos: ## Deploy Chaos Mesh
	$(HELM) repo add chaos-mesh https://charts.chaos-mesh.org 2>/dev/null; true
	$(HELM) upgrade --install chaos-mesh chaos-mesh/chaos-mesh \
		--namespace=chaos-testing --create-namespace \
		--set chaosDaemon.runtime=containerd \
		--set chaosDaemon.socketPath=/run/containerd/containerd.sock \
		--wait

# ─── Chaos Attacks ────────────────────────────────────────────────────────────
attack-network: ## Inject 1200ms gRPC latency between frontend and backend
	$(KUBECTL) apply -f chaos/manifests/network-latency-chaos.yaml

attack-pods: ## Kill 2 of 3 backend pods
	$(KUBECTL) apply -f chaos/manifests/pod-failure-chaos.yaml

stop-chaos: ## Stop all chaos experiments
	-$(KUBECTL) delete chaos -n chaos-testing --all
	-$(KUBECTL) delete podchaos -n resilience --all
	-$(KUBECTL) delete networkchaos -n resilience --all

# ─── Full Demo ────────────────────────────────────────────────────────────────
full-demo: ## Full demo: cluster → build → deploy → k6 → chaos → report
	@echo "=== Step 0: Prerequisites check ==="
	@command -v kind >/dev/null 2>&1 || { echo "kind required"; exit 1; }
	@command -v k6 >/dev/null 2>&1 || { echo "k6 required"; exit 1; }
	@echo "=== Step 1: Create cluster ==="
	$(MAKE) kind-up
	@echo "=== Step 2: Build images ==="
	$(MAKE) build-images
	@echo "=== Step 3: Deploy infra + apps ==="
	$(MAKE) deploy-infra
	$(MAKE) deploy-apps
	@echo "=== Step 4: Deploy observability ==="
	$(MAKE) deploy-observability
	@echo "=== Step 5: Run chaos attack ==="
	bash chaos/scripts/run-chaos-attack.sh

# ─── Cleanup ──────────────────────────────────────────────────────────────────
clean: ## Clean binaries and temp files
	rm -rf bin/
	$(MAKE) kind-down 2>/dev/null; true

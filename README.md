# k8s-resilience

> Демонстрация production-grade resilience-паттернов на Go + Kubernetes + Chaos Engineering.

## Концепция

Двухсервисная архитектура (frontend-api → backend-worker), где каждый уровень содержит
защитные механизмы от падений:

```
      [HTTP :8080]
           │
    ┌──────▼──────┐
    │ frontend-api │  ← Circuit Breaker, gRPC Retries, OTel Tracing
    └──────┬──────┘
           │ [gRPC :50051]
    ┌──────▼──────┐
    │backend-worker│  ← PostgreSQL → Redis Fallback, Graceful Shutdown
    └──┬───────┬──┘
       │       │
  [PostgreSQL] [Redis]
```

При отказе любого компонента система **не падает**, а деградирует graceful — пользователь
всегда получает ответ (возможно, с устаревшими данными).

---

## Структура проекта

```
k8s-resilience/
│
├── api/                              # 📦 Общий proto-контракт
│   └── proto/worker/v1/worker.proto #   gRPC спецификация TaskService
│                                    #   (GetTaskData, CreateTask)
│
├── frontend-api/                     # 🖥️ HTTP API сервер (Go)
│   ├── cmd/frontend/main.go         #   Entry point: HTTP :8080
│   └── internal/
│       ├── config/                  #   Конфигурация из env (envconfig)
│       ├── grpcclient/              #   gRPC клиент + Circuit Breaker + Retries
│       ├── handlers/                #   HTTP хендлеры + health/readiness probes
│       ├── model/                   #   DTO модели
│       └── telemetry/               #   OpenTelemetry инициализация
│
├── backend-worker/                   # ⚙️ gRPC бэкенд (Go)
│   ├── cmd/backend/main.go          #   Entry point: gRPC :50051
│   └── internal/
│       ├── config/                  #   Конфигурация из env
│       ├── db/                      #   PostgreSQL store + Redis cache + Fallback
│       ├── model/                   #   Domain модели
│       ├── server/                  #   gRPC TaskService имплементация
│       └── telemetry/               #   OpenTelemetry инициализация
│
├── deployments/                      # 🚀 Развёртывание
│   ├── docker-compose/              #   Локальная разработка (без K8s)
│   │   └── docker-compose.yaml
│   ├── helm/microservices/          #   Helm-чарт для Kubernetes
│   │   ├── Chart.yaml               #   Метаданные чарта
│   │   ├── values.yaml              #   Значения по умолчанию
│   │   └── templates/               #   Шаблоны ресурсов
│   │       ├── frontend-deployment.yaml
│   │       ├── backend-deployment.yaml
│   │       ├── hpa.yaml             #   HorizontalPodAutoscaler
│   │       ├── pdb.yaml             #   PodDisruptionBudget
│   │       ├── networkpolicy.yaml    #   7 политик zero-trust сети
│   │       ├── secret.yaml           #   DATABASE_URL, REDIS_PASSWORD
│   │       └── ...
│   └── kind/                        #   KIND кластер конфиг
│       └── kind-config.yaml
│
├── observability/                    # 📊 Мониторинг
│   ├── otel-collector-config.yaml   #   OTel Collector → Prometheus + Tempo
│   └── grafana/dashboards/
│       └── resilience-dashboard.json #   4 панели: RPS, латенси, CB, поды
│
├── chaos/                            # 💥 Chaos Engineering
│   ├── manifests/
│   │   ├── network-latency-chaos.yaml # 1200ms задержка gRPC
│   │   └── pod-failure-chaos.yaml     # Kill 2/3 backend подов
│   └── scripts/
│       └── run-chaos-attack.sh        # Автоматизация: k6 + chaos + мониторинг
│
├── k6/                               # 🏋️ Нагрузочное тестирование
│   └── load-test.js                  # 100 VUs, 5 минут
│
├── .github/workflows/ci.yaml        # 🔄 CI/CD: lint → test → build → scan → helm
├── ADR/                              # 📐 Архитектурные решения
│   ├── ADR-001-go-module-structure.md
│   ├── ADR-002-circuit-breaker-and-retry.md
│   └── ADR-003-chaos-mesh-selection.md
│
├── Makefile                          # 🎯 21 target для управления проектом
├── go.work                           # Go workspace (3 модуля)
└── .golangci.yaml                    # Линтер конфигурация
```

---

## Resilience-паттерны (снизу вверх)

### 1. Database Fallback (backend-worker)

```go
// internal/db/fallback.go
// При штатной работе: PostgreSQL
// При падении PG (таймаут 1s): Redis Cache
// При падении всего: ошибка с объяснением
func (s *Store) GetTask(ctx context.Context, id string) (*model.Task, string, error)
```

Поле `data_source` в ответе показывает источник:
- `PostgreSQL` — свежие данные из БД
- `Redis_Cache` — данные из кэша (могут быть немного устаревшими)
- `Circuit_Breaker_Fallback` — сервис недоступен, ответ от circuit breaker

### 2. Circuit Breaker (frontend-api)

```
Состояния:
  CLOSED  → нормальная работа, запросы проходят
  OPEN    → 40% ошибок за 10s → моментальный fallback без запросов сети (5s)
  HALF_OPEN → 1 пробный запрос, если успех → CLOSED
```

### 3. gRPC Retries

Экспоненциальный backoff с jitter'ом (100ms → 200ms → 400ms) на коды
`Unavailable`, `DeadlineExceeded`, `ResourceExhausted`. Максимум 3 попытки.

### 4. Graceful Shutdown

Оба сервиса перехватывают `SIGINT`/`SIGTERM` и завершают работу за 15 секунд:
закрывают соединения, дожидаются активных запросов, останавливают gRPC сервер.

### 5. Self-Healing (Kubernetes)

```
LivenessProbe  →  /healthz (frontend), gRPC health (backend)
ReadinessProbe →  /readyz (проверка gRPC соединения)
HPA            →  CPU > 70% ИЛИ Memory > 80% → масштабирование 2 → 6 подов
PDB            →  minAvailable: 2 (ни один апдейт не уронит ниже 2 реплик)
PodAntiAffinity →  поды раскиданы по разным нодам
```

---

## Сценарии Chaos Engineering

### Сценарий A: Сетевая задержка

```
Что: 1200ms латенси между frontend → backend (таймаут в Go — 1000ms)
Результат: таймаут → Circuit Breaker открывается → frontend возвращает fallback
Проверка: data_source = "Circuit_Breaker_Fallback" в ответах API
```

### Сценарий B: Убийство подов

```
Что: 2 из 3 backend-подов убиты на 45 секунд
Результат: ReplicaSet создаёт новые → readinessProbe не пускает трафик →
           HPA масштабирует → система восстанавливается
Проверка: kubectl get pods -w покажет поды в CrashLoop → Running
```

---

## Быстрый старт

```bash
# 1. Локальная разработка (Docker Compose)
make dev
curl http://localhost:8080/api/v1/tasks/task-1

# 2. KIND кластер (3 ноды)
make kind-up
make build-images
make deploy-infra    # PostgreSQL + Redis
make deploy-apps     # frontend + backend

# 3. Нагрузочный тест + Chaos
make attack-network  # 1200ms задержка
# или
make attack-pods     # убить 2/3 подов

# 4. Всё вместе
make full-demo
```

---

## Технологический стек

| Компонент | Технология |
|-----------|------------|
| Язык | Go 1.26 |
| Протокол | gRPC + Protocol Buffers |
| HTTP | net/http (Go 1.22+ routing) |
| БД | PostgreSQL 17 (pgx/v5) |
| Кэш | Redis 7 (go-redis/v9) |
| Трейсинг | OpenTelemetry (OTLP) |
| Метрики | Prometheus / Grafana |
| Оркестрация | Kubernetes (kind) |
| Пакетирование | Helm v3 |
| Chaos | Chaos Mesh |
| Нагрузка | k6 |
| CI/CD | GitHub Actions + Trivy |

---

## Структура модулей (Go Workspace)

Проект использует `go.work` с тремя модулями:

```
go.work
├── ./api              → github.com/resume/k8s-resilience/api
├── ./frontend-api     → github.com/resume/k8s-resilience/frontend-api
└── ./backend-worker   → github.com/resume/k8s-resilience/backend-worker
```

- **api** — только proto-описания и сгенерированный код, без бизнес-логики
- **frontend-api** — зависит от api (через workspace replace)
- **backend-worker** — зависит от api (через workspace replace)

Локально `replace` директивы в go.mod обеспечивают сборку без публикации модулей.

---

## Лицензия

MIT

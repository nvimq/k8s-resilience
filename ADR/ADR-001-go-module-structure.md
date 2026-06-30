# ADR-001: Go Module Structure

## Status
Accepted

## Context
The project has two microservices (frontend-api, backend-worker) and a shared protobuf definition. We need an efficient way to manage dependencies and share the generated proto code.

## Decision
Use Go 1.26 workspace with `go.work` to manage three local modules:

- `api/` — shared protobuf definitions and generated Go code
- `frontend-api/` — HTTP server + gRPC client
- `backend-worker/` — gRPC server + database layer

Each module has its own `go.mod`. The `api` module is consumed via `replace` directives in the service modules during development, and via Git tags in CI.

## Alternatives Considered
- **Single module** — simpler but couples all packages, breaks `internal/` isolation
- **Vendor proto code** — requires manual sync
- **Buf remote generation** — adds build-time dependency on BSR

## Consequences
- Clean dependency isolation (internal packages are truly private)
- Proto changes are type-checked across services at compile time
- CI needs to resolve the workspace or use `replace` directives

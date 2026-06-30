# ADR-002: Circuit Breaker and Retry Strategy

## Status
Accepted

## Context
The frontend-api service calls backend-worker over gRPC. Network failures, pod restarts, and resource exhaustion can cause transient or persistent errors. We need to prevent cascading failures.

## Decision
Layer two resilience mechanisms:

### gRPC Retry Interceptor
- Library: `github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry`
- Max 3 retries
- Exponential backoff with jitter: initial 100ms, multiplier 2.0
- Retry only on `Unavailable`, `DeadlineExceeded`, `ResourceExhausted`

### Circuit Breaker (gobreaker)
- Library: `github.com/sony/gobreaker`
- Trip condition: failure ratio >= 40% in a 10-second window, minimum 5 requests
- Open state timeout: 5 seconds
- Half-Open: 1 test request
- On open: return immediate fallback response with `Circuit_Breaker_Fallback` data source

## Alternatives Considered
- **Only retries** — would still hammer a dead service for 3 attempts
- **Only circuit breaker** — would miss transient errors that retries handle
- **Hystrix** — archived, no longer maintained

## Consequences
- Frontend never waits more than 5s for backend
- Fallback response allows frontend to remain partially functional during backend outage
- Additional metric: circuit breaker state for observability

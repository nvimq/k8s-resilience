# ADR-003: Chaos Engineering Tool Selection

## Status
Accepted

## Context
We need to validate the resilience features (circuit breaker, DB fallback, HPA, probes) under controlled failure conditions.

## Decision
Use **Chaos Mesh** as the chaos engineering platform.

Rationale:
- First-class support for Kind (used for local testing)
- CRD-native (operates like any Kubernetes resource)
- Variety of fault types: network delay, pod failure, CPU stress, IO latency
- Easy to install via Helm
- Active community and CNCF sandbox project

## Alternatives Considered
- **Litmus Chaos** — more complex setup, requires chaos-operator per experiment
- **Toxiproxy** — only for network-level chaos, no pod/kill scenarios
- **Manual kubectl delete** — not repeatable, not observable

## Consequences
- Two experiment types defined: network latency (1200ms) and pod failure (2/3 replicas)
- Experiments are version-controlled as YAML manifests
- Runbook script automates: steady-state check → k6 load → apply chaos → monitor → cleanup

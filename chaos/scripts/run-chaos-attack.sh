#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

NAMESPACE=${NAMESPACE:-resilience}
CHAOS_DIR="$PROJECT_DIR/manifests"
K6_SCRIPT="$PROJECT_DIR/../k6/load-test.js"
DURATION_SEC=${DURATION_SEC:-90}
CHAOS_TYPE=${1:-network}  # network | pods

echo "======================================"
echo "  K8s Resilience Chaos Attack Runner"
echo "======================================"
echo "Namespace: $NAMESPACE"
echo "Chaos Type: $CHAOS_TYPE"
echo "Duration: ${DURATION_SEC}s"
echo ""

echo "=== Phase 1: Steady State Check ==="
echo "Checking pod health..."
kubectl get pods -n "$NAMESPACE" -o wide
echo ""

echo "Checking HPA status..."
kubectl get hpa -n "$NAMESPACE" || echo "No HPA found"
echo ""

echo "=== Phase 2: Start Load Test ==="
echo "Starting k6 load test in background..."
k6 run "$K6_SCRIPT" \
  --out json=/tmp/k6-results.json \
  --duration "${DURATION_SEC}s" \
  --vus 100 &
K6_PID=$!
echo "k6 PID: $K6_PID"
sleep 5

echo "=== Phase 3: Inject Chaos ==="
case "$CHAOS_TYPE" in
  network)
    echo "Injecting 1200ms gRPC latency..."
    kubectl apply -f "$CHAOS_DIR/network-latency-chaos.yaml"
    ;;
  pods)
    echo "Killing 2 of 3 backend pods..."
    kubectl apply -f "$CHAOS_DIR/pod-failure-chaos.yaml"
    ;;
  *)
    echo "Unknown chaos type: $CHAOS_TYPE"
    exit 1
    ;;
esac

echo ""
echo "=== Phase 4: Real-Time Monitoring ==="
echo "Watching pods (Ctrl+C to stop watching, chaos will continue)..."
kubectl get pods -n "$NAMESPACE" -w &
WATCH_PID=$!

sleep "$DURATION_SEC"

echo ""
echo "=== Phase 5: Stop Chaos ==="
kubectl delete chaos --all -n "$NAMESPACE" 2>/dev/null || true
kubectl delete networkchaos --all -n "$NAMESPACE" 2>/dev/null || true
kubectl delete podchaos --all -n "$NAMESPACE" 2>/dev/null || true

echo "Waiting for recovery..."
sleep 10
kubectl get pods -n "$NAMESPACE" -o wide

echo ""
echo "=== Phase 6: Results Summary ==="
kill "$WATCH_PID" 2>/dev/null || true
wait "$K6_PID" 2>/dev/null || true

if [ -f /tmp/k6-results.json ]; then
  echo "k6 results saved to /tmp/k6-results.json"
  echo ""
  echo "Fallback rate (circuit breaker hits):"
  grep -c '"data_source":"Circuit_Breaker_Fallback"' /tmp/k6-results.json 2>/dev/null || echo "0"
  echo "Redis cache hits:"
  grep -c '"data_source":"Redis_Cache"' /tmp/k6-results.json 2>/dev/null || echo "0"
fi

echo ""
echo "=== Chaos Attack Complete ==="
echo "System should be fully recovered."
kubectl get pods -n "$NAMESPACE"

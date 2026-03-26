#!/bin/bash
# auto-rollback.sh — Run as a post-deploy health check in CI/CD
# If any pods are in CrashLoopBackOff after deploy, roll back the Helm release.

set -e

RELEASE_NAME="${1:-app-a}"
WAIT_SECONDS="${2:-30}"
NAMESPACE="${3:-default}"

echo "Waiting ${WAIT_SECONDS}s for pods to stabilize after deploy..."
sleep "$WAIT_SECONDS"

echo "Checking pod health for release: $RELEASE_NAME..."
CRASH_COUNT=$(kubectl get pods -n "$NAMESPACE" -l "app=$RELEASE_NAME" --no-headers 2>/dev/null \
  | grep -c "CrashLoopBackOff" || true)

FAILED_COUNT=$(kubectl get pods -n "$NAMESPACE" -l "app=$RELEASE_NAME" \
  --field-selector=status.phase=Failed --no-headers 2>/dev/null | wc -l | tr -d ' ')

if [ "$CRASH_COUNT" -gt 0 ] || [ "$FAILED_COUNT" -gt 0 ]; then
  echo "ALERT: Unhealthy pods detected (CrashLoopBackOff=$CRASH_COUNT, Failed=$FAILED_COUNT)"
  echo "Rolling back Helm release '$RELEASE_NAME'..."
  helm rollback "$RELEASE_NAME" --wait
  echo ""
  echo "Rollback complete. Helm history:"
  helm history "$RELEASE_NAME"
  exit 1
fi

echo "All pods healthy. No rollback needed."
kubectl get pods -n "$NAMESPACE" -l "app=$RELEASE_NAME"

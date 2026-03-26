#!/usr/bin/env bash
# nginx-rollback-canary.sh — Remove the Nginx canary ingress, service, and Helm release.
#
# Usage:
#   ./canary/nginx-rollback-canary.sh [STABLE_RELEASE] [NAMESPACE]

set -euo pipefail

STABLE_RELEASE="${1:-app-a}"
NAMESPACE="${2:-default}"
CANARY_RELEASE="${STABLE_RELEASE}-canary"
SCRIPT_DIR="$(dirname "$0")"

echo "======================================="
echo " Nginx Canary Rollback"
echo "======================================="
echo "  Canary release : $CANARY_RELEASE"
echo "  Namespace      : $NAMESPACE"
echo ""

echo "Step 1/3: Removing canary ingress..."
kubectl delete -f "$SCRIPT_DIR/nginx-ingress-canary.yaml" -n "$NAMESPACE" --ignore-not-found

echo "Step 2/3: Removing canary service..."
kubectl delete -f "$SCRIPT_DIR/nginx-service-canary.yaml" -n "$NAMESPACE" --ignore-not-found

echo "Step 3/3: Removing canary Helm release..."
if helm status "$CANARY_RELEASE" -n "$NAMESPACE" &>/dev/null; then
  helm uninstall "$CANARY_RELEASE" --namespace "$NAMESPACE"
else
  echo "  Canary release '$CANARY_RELEASE' already gone."
fi

echo ""
echo "Rollback complete. 100% of traffic now routes to stable."
kubectl get pods -n "$NAMESPACE" -l "app=${STABLE_RELEASE}" -o wide

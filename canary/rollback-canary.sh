#!/usr/bin/env bash
# rollback-canary.sh — Remove a canary release without touching stable.
#
# Usage:
#   ./canary/rollback-canary.sh [STABLE_RELEASE] [NAMESPACE]
#
# Use this when canary validation fails and you want to route all traffic
# back to the stable release immediately.

set -euo pipefail

STABLE_RELEASE="${1:-app-a}"
NAMESPACE="${2:-default}"
CANARY_RELEASE="${STABLE_RELEASE}-canary"

echo "=============================="
echo " Rolling Back Canary"
echo "=============================="
echo "  Canary release : $CANARY_RELEASE"
echo "  Namespace      : $NAMESPACE"
echo ""

if ! helm status "$CANARY_RELEASE" -n "$NAMESPACE" &>/dev/null; then
  echo "Canary release '$CANARY_RELEASE' not found — nothing to roll back."
  exit 0
fi

echo "Deleting canary release '$CANARY_RELEASE'..."
helm uninstall "$CANARY_RELEASE" --namespace "$NAMESPACE"

echo ""
echo "Canary rolled back. Verifying stable pods..."
kubectl get pods -n "$NAMESPACE" -l "app=$STABLE_RELEASE" -o wide

echo ""
echo "All traffic is now routed exclusively to the stable release."

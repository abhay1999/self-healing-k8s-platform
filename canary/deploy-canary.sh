#!/usr/bin/env bash
# deploy-canary.sh — Deploy a canary release alongside the stable deployment.
#
# Usage:
#   ./canary/deploy-canary.sh <IMAGE_TAG> [STABLE_RELEASE] [NAMESPACE]
#
# Examples:
#   ./canary/deploy-canary.sh abc1234
#   ./canary/deploy-canary.sh abc1234 app-a production
#
# Traffic split:
#   1 canary pod / (stable_replicas + 1) ≈ 10% canary traffic (with stable at 9 replicas)
#   Adjust stable replicas before deploying to control the split percentage.

set -euo pipefail

IMAGE_TAG="${1:?Usage: $0 <image-tag> [stable-release] [namespace]}"
STABLE_RELEASE="${2:-app-a}"
NAMESPACE="${3:-default}"
CANARY_RELEASE="${STABLE_RELEASE}-canary"
CHART_DIR="$(dirname "$0")/../helm-chart"
VALUES_FILE="$(dirname "$0")/values-canary.yaml"

echo "=============================="
echo " Canary Deployment"
echo "=============================="
echo "  Image tag      : $IMAGE_TAG"
echo "  Stable release : $STABLE_RELEASE"
echo "  Canary release : $CANARY_RELEASE"
echo "  Namespace      : $NAMESPACE"
echo ""

# Verify stable release exists
if ! helm status "$STABLE_RELEASE" -n "$NAMESPACE" &>/dev/null; then
  echo "ERROR: Stable release '$STABLE_RELEASE' not found in namespace '$NAMESPACE'."
  echo "Deploy stable first: helm install $STABLE_RELEASE ./helm-chart"
  exit 1
fi

STABLE_REPLICAS=$(kubectl get deployment "$STABLE_RELEASE" -n "$NAMESPACE" \
  -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "2")
echo "Stable replicas  : $STABLE_REPLICAS"
echo "Canary replicas  : 1"
echo "Traffic to canary: ~$(echo "scale=0; 100 / ($STABLE_REPLICAS + 1)" | bc)%"
echo ""

# Deploy (or upgrade) canary release
helm upgrade --install "$CANARY_RELEASE" "$CHART_DIR" \
  -f "$VALUES_FILE" \
  --set image.tag="$IMAGE_TAG" \
  --set appName="$STABLE_RELEASE" \
  --namespace "$NAMESPACE" \
  --wait --timeout 2m

echo ""
echo "Canary deployed. Verifying pods..."
kubectl get pods -n "$NAMESPACE" \
  -l "app=${STABLE_RELEASE}" \
  -o wide

echo ""
echo "Next steps:"
echo "  Monitor  : kubectl get pods -n $NAMESPACE -l app=$STABLE_RELEASE -w"
echo "  Promote  : ./canary/promote-canary.sh $IMAGE_TAG $STABLE_RELEASE $NAMESPACE"
echo "  Rollback : ./canary/rollback-canary.sh $STABLE_RELEASE $NAMESPACE"

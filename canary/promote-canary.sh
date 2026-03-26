#!/usr/bin/env bash
# promote-canary.sh — Promote canary to stable and remove the canary release.
#
# Usage:
#   ./canary/promote-canary.sh <IMAGE_TAG> [STABLE_RELEASE] [NAMESPACE]
#
# What this does:
#   1. Upgrades the stable Helm release to the canary image tag
#   2. Waits for the stable rollout to complete
#   3. Deletes the canary Helm release

set -euo pipefail

IMAGE_TAG="${1:?Usage: $0 <image-tag> [stable-release] [namespace]}"
STABLE_RELEASE="${2:-app-a}"
NAMESPACE="${3:-default}"
CANARY_RELEASE="${STABLE_RELEASE}-canary"
CHART_DIR="$(dirname "$0")/../helm-chart"

echo "=============================="
echo " Promoting Canary → Stable"
echo "=============================="
echo "  Image tag      : $IMAGE_TAG"
echo "  Stable release : $STABLE_RELEASE"
echo "  Namespace      : $NAMESPACE"
echo ""

# Verify canary release exists
if ! helm status "$CANARY_RELEASE" -n "$NAMESPACE" &>/dev/null; then
  echo "WARNING: Canary release '$CANARY_RELEASE' not found. Nothing to promote from."
  echo "Proceeding to upgrade stable directly..."
fi

echo "Step 1/2: Upgrading stable release to image tag '$IMAGE_TAG'..."
helm upgrade "$STABLE_RELEASE" "$CHART_DIR" \
  --reuse-values \
  --set image.tag="$IMAGE_TAG" \
  --namespace "$NAMESPACE" \
  --wait --timeout 3m

echo ""
echo "Step 2/2: Removing canary release..."
if helm status "$CANARY_RELEASE" -n "$NAMESPACE" &>/dev/null; then
  helm uninstall "$CANARY_RELEASE" --namespace "$NAMESPACE"
  echo "Canary release '$CANARY_RELEASE' removed."
else
  echo "Canary release '$CANARY_RELEASE' already gone."
fi

echo ""
echo "Promotion complete. All traffic now routes to stable ($IMAGE_TAG)."
echo ""
kubectl get pods -n "$NAMESPACE" -l "app=$STABLE_RELEASE" -o wide
echo ""
helm history "$STABLE_RELEASE" -n "$NAMESPACE" --max 5

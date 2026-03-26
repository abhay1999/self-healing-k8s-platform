#!/usr/bin/env bash
# nginx-deploy-canary.sh — Deploy a canary with Nginx ingress weight-based traffic splitting.
#
# Unlike the replica-ratio approach (deploy-canary.sh), this gives exact
# percentage control (e.g. exactly 10%) regardless of replica counts.
#
# Usage:
#   ./canary/nginx-deploy-canary.sh <IMAGE_TAG> [CANARY_WEIGHT] [STABLE_RELEASE] [NAMESPACE]
#
# Examples:
#   ./canary/nginx-deploy-canary.sh abc1234
#   ./canary/nginx-deploy-canary.sh abc1234 20          # 20% to canary
#   ./canary/nginx-deploy-canary.sh abc1234 10 app-a production

set -euo pipefail

IMAGE_TAG="${1:?Usage: $0 <image-tag> [canary-weight] [stable-release] [namespace]}"
CANARY_WEIGHT="${2:-10}"
STABLE_RELEASE="${3:-app-a}"
NAMESPACE="${4:-default}"
CANARY_RELEASE="${STABLE_RELEASE}-canary"
CHART_DIR="$(dirname "$0")/../helm-chart"
VALUES_FILE="$(dirname "$0")/values-canary.yaml"
SCRIPT_DIR="$(dirname "$0")"

echo "======================================="
echo " Nginx Ingress Canary Deployment"
echo "======================================="
echo "  Image tag      : $IMAGE_TAG"
echo "  Canary weight  : ${CANARY_WEIGHT}%"
echo "  Stable release : $STABLE_RELEASE"
echo "  Canary release : $CANARY_RELEASE"
echo "  Namespace      : $NAMESPACE"
echo ""

# Check nginx ingress controller is available
if ! kubectl get ingressclass nginx &>/dev/null; then
  echo "Nginx ingress controller not found. Enabling Minikube addon..."
  minikube addons enable ingress
  echo "Waiting for ingress controller to be ready..."
  kubectl wait --namespace ingress-nginx \
    --for=condition=ready pod \
    --selector=app.kubernetes.io/component=controller \
    --timeout=90s
fi

# Step 1: deploy canary pods via Helm
echo "Step 1/4: Deploying canary pods..."
helm upgrade --install "$CANARY_RELEASE" "$CHART_DIR" \
  -f "$VALUES_FILE" \
  --set image.tag="$IMAGE_TAG" \
  --set appName="$STABLE_RELEASE" \
  --namespace "$NAMESPACE" \
  --wait --timeout 2m

# Step 2: create dedicated canary service (selects track=canary pods only)
echo ""
echo "Step 2/4: Applying canary service..."
kubectl apply -f "$SCRIPT_DIR/nginx-service-canary.yaml" -n "$NAMESPACE"

# Step 3: apply stable ingress (if not already present)
echo ""
echo "Step 3/4: Applying stable ingress..."
kubectl apply -f "$SCRIPT_DIR/nginx-ingress-stable.yaml" -n "$NAMESPACE"

# Step 4: apply canary ingress with the requested weight
echo ""
echo "Step 4/4: Applying canary ingress (weight=${CANARY_WEIGHT}%)..."
kubectl apply -f "$SCRIPT_DIR/nginx-ingress-canary.yaml" -n "$NAMESPACE"

# Update weight if different from default (10)
if [ "$CANARY_WEIGHT" != "10" ]; then
  kubectl annotate ingress app-a-canary \
    "nginx.ingress.kubernetes.io/canary-weight=${CANARY_WEIGHT}" \
    --overwrite -n "$NAMESPACE"
fi

echo ""
echo "Nginx canary deployed."
echo ""
kubectl get pods -n "$NAMESPACE" -l "app=${STABLE_RELEASE}" -o wide
echo ""
kubectl get ingress -n "$NAMESPACE"
echo ""

MINIKUBE_IP=$(minikube ip 2>/dev/null || echo "<minikube-ip>")
echo "Add to /etc/hosts if not already present:"
echo "  echo '${MINIKUBE_IP} app.local' | sudo tee -a /etc/hosts"
echo ""
echo "Test traffic split (run ~20 times, expect ~${CANARY_WEIGHT}% canary responses):"
echo "  for i in \$(seq 1 20); do curl -s http://app.local/ | jq -r .service; done | sort | uniq -c"
echo ""
echo "Adjust weight  : kubectl annotate ingress app-a-canary nginx.ingress.kubernetes.io/canary-weight=25 --overwrite -n $NAMESPACE"
echo "Promote canary : ./canary/promote-canary.sh $IMAGE_TAG $STABLE_RELEASE $NAMESPACE"
echo "Rollback canary: ./canary/nginx-rollback-canary.sh $STABLE_RELEASE $NAMESPACE"

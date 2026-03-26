#!/bin/bash
# Run this script on Day 8 to install the full observability stack
set -e

echo "Adding Helm repos..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm repo update

echo "Creating monitoring namespace..."
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

echo "Installing Prometheus + Grafana (kube-prometheus-stack)..."
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring \
  -f observability/prometheus-values.yaml \
  --wait

echo "Installing Jaeger..."
helm upgrade --install jaeger jaegertracing/jaeger \
  -n monitoring \
  -f observability/jaeger-values.yaml \
  --wait

echo ""
echo "Done! Access the UIs with:"
echo "  Prometheus: kubectl port-forward svc/prometheus-operated 9090:9090 -n monitoring"
echo "  Grafana:    kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring  (admin / admin123)"
echo "  Jaeger:     kubectl port-forward svc/jaeger-query 16686:16686 -n monitoring"

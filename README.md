# Self-Healing Kubernetes Platform

![CI/CD](https://github.com/yourusername/self-healing-k8s-platform/actions/workflows/ci-cd.yaml/badge.svg)
![Go Version](https://img.shields.io/badge/Go-1.22-blue)
![Kubernetes](https://img.shields.io/badge/Kubernetes-1.29-blue)

A production-grade, self-healing Kubernetes platform built from scratch. Demonstrates CI/CD automation, full observability (metrics, dashboards, tracing), automatic failure recovery, chaos engineering, canary deployments, and a custom Kubernetes controller — all deployed on a local Minikube cluster.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        GITHUB ACTIONS CI/CD                         │
│  push → build image → push to Docker Hub → helm upgrade → health   │
│                         check → auto-rollback if unhealthy          │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ helm upgrade / rollback
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        KUBERNETES CLUSTER (Minikube)                │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  Service: app-a  (selector: app=app-a)                       │   │
│  │  Routes traffic to stable + canary pods by replica ratio     │   │
│  └────────────┬───────────────────────────┬─────────────────────┘   │
│               │ ~90%                      │ ~10%                    │
│  ┌────────────▼──────────┐  ┌─────────────▼──────────┐            │
│  │  Stable Deployment    │  │  Canary Deployment      │            │
│  │  track: stable        │  │  track: canary          │            │
│  │  replicas: 9          │  │  replicas: 1            │            │
│  │  image: v1.0          │  │  image: v2.0 (new)      │            │
│  │                       │  │                         │            │
│  │  /health  /ready      │  │  /health  /ready        │            │
│  │  /metrics /crash      │  │  /metrics /crash        │            │
│  └───────────────────────┘  └─────────────────────────┘            │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  Self-Healing Controller (custom Go controller)              │   │
│  │  Watches all pods → restarts > 3 times → delete + reschedule │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  Kubernetes Native Self-Healing                              │   │
│  │  • Liveness probe  → restart container on /health failure    │   │
│  │  • Readiness probe → remove from service on /ready failure   │   │
│  │  • HPA             → scale 2–5 replicas at 70% CPU          │   │
│  │  • RollingUpdate   → zero-downtime deploys                   │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌──────────────────────── monitoring namespace ────────────────┐   │
│  │  Prometheus :9090   Grafana :3000   AlertManager :9093       │   │
│  │  Jaeger :16686 (distributed tracing via OpenTelemetry)       │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

| Layer | Tools |
|---|---|
| CI/CD Pipeline | GitHub Actions + Docker Hub |
| Kubernetes Orchestration | Minikube + Helm |
| Microservice | Go (HTTP server with Prometheus metrics + OpenTelemetry traces) |
| Metrics | Prometheus (kube-prometheus-stack) |
| Dashboards | Grafana |
| Distributed Tracing | Jaeger |
| Self-Healing | Liveness/Readiness Probes, Helm auto-rollback, Custom Controller |
| Canary Deployments | Helm-based label traffic splitting with promote/rollback scripts |
| Chaos Engineering | Random pod kill script |

---

## Quick Start

### Prerequisites

```bash
brew install docker kubectl helm minikube go
```

### 1. Start Minikube

```bash
minikube start --driver=docker --memory=4096 --cpus=2
kubectl get nodes   # should show 1 node Ready
```

### 2. Deploy the App

```bash
# Point Docker at Minikube's registry
eval $(minikube docker-env)

# Build image locally
docker build -t self-healing-app:latest ./app

# Deploy via Helm
helm install app-a ./helm-chart \
  --set image.repository=self-healing-app \
  --set image.tag=latest \
  --set image.pullPolicy=Never

kubectl get pods   # Should show 2 Running pods
```

### 3. Access the App

```bash
kubectl port-forward svc/app-a 8080:80
curl http://localhost:8080/         # {"status":"ok","service":"app-a"}
curl http://localhost:8080/health   # {"status":"healthy"}
curl http://localhost:8080/metrics  # Prometheus metrics
```

### 4. Install Observability Stack

```bash
./observability/install.sh

# Access UIs:
kubectl port-forward svc/prometheus-operated 9090:9090 -n monitoring
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring    # admin/admin123
kubectl port-forward svc/jaeger-query 16686:16686 -n monitoring
```

---

## Self-Healing Demo

### Simulate a crash

```bash
# Hit the crash endpoint — pod will exit with code 1
curl http://localhost:8080/crash

# Watch K8s restart it automatically (liveness probe detects failure)
kubectl get pods -w
```

Expected output:
```
NAME                     READY   STATUS             RESTARTS
app-a-xxxx               0/1     CrashLoopBackOff   1
app-a-xxxx               1/1     Running            2      # <-- auto-restarted!
```

### Simulate a bad deploy + auto-rollback

```bash
# Deploy a broken image
helm upgrade app-a ./helm-chart --set image.tag=broken-does-not-exist

# Run the rollback script
./self-healing/auto-rollback.sh app-a 30

# Check Helm history
helm history app-a
```

---

## Canary Deployment

Two strategies are available. Both use the same `track` label system on pods.

### Strategy A — Replica-ratio (no extra dependencies)

Traffic splits by replica count ratio. Simple, works on bare Minikube with no ingress controller.

```
Service: app-a  (selector: app=app-a — matches both tracks)
    ├── Stable pods  (track=stable, 9 replicas) ← ~90% traffic
    └── Canary pods  (track=canary, 1 replica)  ←  ~10% traffic
```

```bash
./canary/deploy-canary.sh <new-image-tag>          # deploy ~10% canary
./canary/promote-canary.sh <new-image-tag>          # promote: upgrade stable, remove canary
./canary/rollback-canary.sh                         # rollback: remove canary instantly
```

### Strategy B — Nginx Ingress weight (exact percentage)

Nginx routes exactly N% of requests to the canary backend regardless of replica counts. Requires the Nginx ingress controller (enabled automatically by the script).

```
Nginx Ingress
    ├── app-a (stable ingress)  ← 90% of requests
    └── app-a-canary (canary ingress, weight=10) ← exactly 10% of requests
              └── app-a-canary Service  (selector: app=app-a, track=canary)
                        └── Canary pods
```

```bash
# Deploy with exact 10% weight
./canary/nginx-deploy-canary.sh <new-image-tag>

# Deploy with custom weight (e.g. 25%)
./canary/nginx-deploy-canary.sh <new-image-tag> 25

# Adjust weight live (no redeploy needed)
kubectl annotate ingress app-a-canary \
  nginx.ingress.kubernetes.io/canary-weight=25 --overwrite

# Verify the split (run ~20 times, count canary vs stable responses)
for i in $(seq 1 20); do curl -s http://app.local/ | jq -r .service; done | sort | uniq -c

# Promote or rollback
./canary/promote-canary.sh <new-image-tag>
./canary/nginx-rollback-canary.sh
```

### Validate either strategy

```bash
# Watch pods by track label
kubectl get pods -l app=app-a -o wide -w

# Tail canary logs specifically
kubectl logs -l "app=app-a,track=canary" -f

# Watch Grafana dashboards for error rate / latency delta
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring
```

---

## Chaos Testing

Run the chaos script while watching Grafana dashboards:

```bash
./chaos-testing/kill-pods.sh 30 app-a   # kill a random pod every 30 seconds
```

Open Grafana at `localhost:3000` and watch the pod restart count — traffic should never fully drop.

---

## Custom Controller

The Go controller in `controller/` watches all pods. When a pod restarts more than 3 times, it deletes and forces a fresh replacement:

```bash
# Build and deploy the controller
cd controller
docker build -t self-healing-controller:latest .
kubectl apply -f deploy.yaml

# Watch controller logs
kubectl logs -f deployment/self-healing-controller
```

---

## CI/CD Pipeline

Every push to `main` triggers:

1. **Build** — Docker image built from `./app`
2. **Push** — Image pushed to Docker Hub tagged with commit SHA
3. **Deploy** — `helm upgrade` deploys new image to K8s
4. **Health Check** — Script checks for CrashLoopBackOff; auto-rolls back if found

Add these GitHub Secrets to enable it:
- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`
- `KUBECONFIG` (base64-encoded kubeconfig: `cat ~/.kube/config | base64`)

---

## Project Structure

```
self-healing-k8s-platform/
├── app/                        # Go microservice
│   ├── main.go                 # HTTP server + Prometheus metrics + Jaeger traces
│   ├── Dockerfile              # Multi-stage build
│   └── go.mod
├── helm-chart/                 # Helm chart (supports stable + canary releases)
│   ├── Chart.yaml
│   ├── values.yaml             # Liveness/readiness probes, HPA, canary flags
│   └── templates/
│       ├── deployment.yaml     # Pods labelled with app + track for traffic splitting
│       ├── service.yaml        # Selects by app only — routes to stable AND canary
│       └── hpa.yaml            # Disabled automatically for canary releases
├── canary/                     # Canary deployment workflow
│   ├── values-canary.yaml      # Helm values for canary release (1 replica, no service/HPA)
│   ├── deploy-canary.sh        # Strategy A: replica-ratio canary deploy
│   ├── promote-canary.sh       # Promote canary image to stable, remove canary release
│   ├── rollback-canary.sh      # Strategy A: remove canary, restore 100% stable traffic
│   ├── nginx-deploy-canary.sh  # Strategy B: Nginx ingress exact-weight canary deploy
│   ├── nginx-rollback-canary.sh # Strategy B: remove canary ingress + service + release
│   ├── nginx-ingress-stable.yaml  # Stable Nginx ingress manifest
│   ├── nginx-ingress-canary.yaml  # Canary Nginx ingress (canary-weight annotation)
│   └── nginx-service-canary.yaml  # Dedicated service selecting track=canary pods only
├── observability/
│   ├── prometheus-values.yaml  # kube-prometheus-stack config
│   ├── jaeger-values.yaml      # Jaeger tracing config
│   └── install.sh              # One-shot observability stack installer
├── self-healing/
│   ├── alertmanager-rules.yaml # CrashLoopBackOff + latency alerts
│   └── auto-rollback.sh        # Post-deploy health check + rollback script
├── controller/
│   ├── main.go                 # Custom K8s controller (watches pods, auto-heals)
│   ├── deploy.yaml             # Controller deployment + RBAC
│   └── go.mod
├── chaos-testing/
│   └── kill-pods.sh            # Random pod killer for chaos experiments
├── .github/workflows/
│   └── ci-cd.yaml              # GitHub Actions pipeline
└── README.md
```

---

## Key Concepts Demonstrated

| Concept | Implementation |
|---|---|
| **Liveness Probes** | K8s restarts containers that fail `/health` checks |
| **Readiness Probes** | Traffic only routes to pods that pass `/ready` |
| **Helm Rollbacks** | One command to revert to any previous deployment |
| **Custom Controller** | Go program using `controller-runtime` to extend K8s with custom healing logic |
| **Prometheus Metrics** | Custom counters and histograms exported from Go app |
| **Grafana Dashboards** | Real-time visualization of cluster and app health |
| **Distributed Tracing** | Every request traced end-to-end with Jaeger + OpenTelemetry |
| **Chaos Engineering** | Deliberate failure injection to validate system resilience |
| **Pod Disruption Budget** | Guarantees minimum pod availability during node drains and voluntary disruptions |
| **Canary Deployments** | Two strategies: replica-ratio (no deps) and Nginx ingress exact-weight splitting |

---

## Self-Healing Mechanisms Summary

| Mechanism | Level | Trigger | Action |
|---|---|---|---|
| Liveness Probe | K8s Native | `/health` fails 3× | Restart container |
| Readiness Probe | K8s Native | `/ready` fails 2× | Remove pod from service |
| Custom Controller | Application | Restarts > 3 | Delete pod → reschedule fresh |
| Helm Auto-Rollback | CI/CD | CrashLoopBackOff post-deploy | `helm rollback` to last good release |
| HPA | K8s Native | CPU > 70% | Scale up to 5 replicas |
| Rolling Update | K8s Native | New deployment | Zero-downtime: maxSurge=1, maxUnavailable=0 |
| Pod Disruption Budget | K8s Native | Node drain / voluntary disruption | Block disruption if it drops below minAvailable |
| Canary Rollback | Deployment | Bad canary validation | Remove canary → 100% stable traffic |

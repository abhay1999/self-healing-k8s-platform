# Demo Recording Script

This script walks through every feature in a logical order for a ~10 minute screen recording.

**Recommended tool:** [Loom](https://loom.com), QuickTime, or OBS
**Suggested layout:** Split terminal (left) + browser (right)

---

## Setup Before Recording

Run these commands silently before you hit record:

```bash
# 1. Start cluster
minikube start --driver=docker --memory=4096 --cpus=2

# 2. Build and deploy
eval $(minikube docker-env)
docker build -t self-healing-app:latest ./app
helm install app-a ./helm-chart \
  --set image.repository=self-healing-app \
  --set image.tag=latest \
  --set image.pullPolicy=Never

# 3. Install observability
./observability/install.sh

# 4. Build and deploy controller
docker build -t self-healing-controller:latest ./controller
kubectl apply -f controller/deploy.yaml

# 5. Pre-forward ports in background
kubectl port-forward svc/app-a 8080:80 &
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring &
kubectl port-forward svc/jaeger-query 16686:16686 -n monitoring &

# 6. Build v2 image for canary demo later
docker tag self-healing-app:latest self-healing-app:v2
```

Open these tabs in your browser before recording:
- `http://localhost:3000` — Grafana (admin / admin123)
- `http://localhost:16686` — Jaeger
- `https://github.com/abhay1999/self-healing-k8s-platform` — GitHub repo

---

## Scene 1 — Introduction (1 min)

**Show:** GitHub repo README
**Say:** "This is a self-healing Kubernetes platform I built from scratch over 30 days. The core idea: in production, things break — pods crash, bad deployments happen, traffic spikes hit. Instead of waking up at 3am, this platform detects and recovers from failures automatically."

**Show:** The architecture Mermaid diagram in the README
**Highlight:** The 7 recovery layers listed in the table

---

## Scene 2 — The Running App (1 min)

**Show:** Terminal
**Type and explain each command:**

```bash
# Show the cluster is running
kubectl get nodes

# Show 2 pods running
kubectl get pods -o wide

# Show the app responding
curl http://localhost:8080/
curl http://localhost:8080/health
curl http://localhost:8080/metrics | head -20
```

**Say:** "The app is a Go HTTP server. It exposes a `/health` endpoint for liveness probes, `/ready` for readiness probes, and `/metrics` for Prometheus scraping."

---

## Scene 3 — Self-Healing: Pod Crash (2 min)

**Show:** Split view — `kubectl get pods -w` on left, terminal on right

```bash
# Left terminal
kubectl get pods -w

# Right terminal — trigger the crash endpoint
curl http://localhost:8080/crash
```

**Point out:** Pod goes `Running → CrashLoopBackOff → Running` within seconds
**Say:** "The liveness probe hits `/health` every 5 seconds. After 3 failures, Kubernetes automatically restarts the container. No human involved."

```bash
# Show restart count
kubectl get pods
# RESTARTS column shows 1
```

---

## Scene 4 — Custom Controller Healing (1 min)

**Show:** Controller logs in one pane, trigger more crashes in another

```bash
# Watch controller
kubectl logs -f deployment/self-healing-controller

# In another pane — crash the pod multiple times (repeat 4x)
curl http://localhost:8080/crash; sleep 3
curl http://localhost:8080/crash; sleep 3
curl http://localhost:8080/crash; sleep 3
curl http://localhost:8080/crash
```

**Point out:** When restarts exceed 3, the controller log shows:
```
[SELF-HEAL] Pod default/app-a-xxx container app has restarted 4 times — deleting pod
[SELF-HEAL] Deleted pod default/app-a-xxx — K8s will schedule a fresh replacement
```

**Say:** "This custom Go controller I wrote watches every pod in the cluster. When a pod is stuck in a restart loop beyond a threshold, it deletes it to force a completely fresh start — different from a simple container restart."

---

## Scene 5 — Observability (1.5 min)

**Show:** Switch to Grafana browser tab

- Go to **Dashboards → Kubernetes / Pods**
- Point out: CPU, memory, restart count, pod count
- Open **Explore**, type query: `http_requests_total`
- Show the counter incrementing with each curl

**Show:** Switch to Jaeger tab

```bash
# Generate some traffic first
for i in $(seq 1 10); do curl -s http://localhost:8080/ > /dev/null; done
```

- In Jaeger: Service = `self-healing-app` → Find Traces
- Click a trace → show the span timeline
- **Say:** "Every single HTTP request is traced end-to-end with OpenTelemetry. In production you'd use this to debug latency spikes across microservices."

---

## Scene 6 — Bad Deploy + Auto-Rollback (1.5 min)

**Show:** Terminal side-by-side with `kubectl get pods -w`

```bash
# Left: watch pods
kubectl get pods -w

# Right: deploy a broken image
helm upgrade app-a ./helm-chart \
  --set image.repository=self-healing-app \
  --set image.tag=does-not-exist \
  --set image.pullPolicy=Never
```

**Point out:** Pods enter `ErrImageNeverPull` or `CrashLoopBackOff`

```bash
# Run the rollback script
./self-healing/auto-rollback.sh app-a 30
```

**Show:**
```bash
helm history app-a
# REVISION 1: deployed (good)
# REVISION 2: failed
# REVISION 3: deployed (rollback)
```

**Say:** "The auto-rollback script is also baked into the GitHub Actions pipeline. Every deployment is automatically health-checked, and if it finds crashed pods, it rolls back without any human action."

---

## Scene 7 — Chaos Testing (1 min)

**Show:** Grafana dashboard open, two terminals side by side

```bash
# Terminal 1: continuous traffic (should never stop)
watch -n 0.5 'curl -s http://localhost:8080/ | jq .status'

# Terminal 2: kill a pod every 10 seconds
./chaos-testing/kill-pods.sh 10 app-a
```

**Point out:**
- Terminal 1 always shows `"ok"` — zero dropped requests
- Grafana shows restart count rising but request rate stays flat

**Say:** "This simulates a real failure scenario. Kubernetes reschedules replacement pods fast enough that the service never goes down. The PodDisruptionBudget also ensures this works safely during planned maintenance like node drains."

```bash
# Stop chaos
Ctrl+C
```

---

## Scene 8 — Canary Deployment (1.5 min)

**Show:** Terminal

```bash
# Deploy 10% canary traffic
./canary/deploy-canary.sh v2

# Show both stable and canary pods
kubectl get pods -l app=app-a -o wide
```

```bash
# Prove the traffic split — run 20 requests
for i in $(seq 1 20); do curl -s http://localhost:8080/ | jq -r .service; done | sort | uniq -c
#  18 app-a         ← stable
#   2 app-a-canary  ← ~10% canary
```

**Say:** "This is a real production pattern. You deploy the new version to 1 pod, verify it works on real traffic, then promote it. If something looks wrong in Grafana, you roll back with a single command."

```bash
# Show rollback
./canary/rollback-canary.sh

kubectl get pods -l app=app-a
# Only stable pods remain
```

---

## Scene 9 — Unit Tests (30 sec)

**Show:** Terminal

```bash
cd app && go test ./... -v
cd ../controller && go test ./... -v
```

**Point out:** All tests pass — green output
**Say:** "The app handlers and the controller reconciler both have full unit test coverage. The controller tests use a fake Kubernetes client so they run without a real cluster."

---

## Scene 10 — Wrap-up (30 sec)

**Show:** GitHub repo README, scroll through it

**Say:** "Everything you just saw — all the code, Helm charts, scripts, tests, and this README — is on GitHub. The platform covers: CI/CD, observability, 7 layers of self-healing, chaos engineering, and canary deployments. Built in 30 days using Go, Kubernetes, Helm, Prometheus, Grafana, Jaeger, and GitHub Actions."

---

## Recording Tips

- Use font size 16+ in terminal for readability
- Run `clear` between scenes
- Pause 1 second after typing a command before pressing Enter — gives viewers time to read
- If a command has long output, pipe to `| head -20` or `| tail -20`
- Record at 1920×1080, export at 1080p
- Add chapter markers in the video description matching the scenes above

## Suggested Video Title

`Self-Healing Kubernetes Platform | Go + Prometheus + Grafana + Jaeger + Canary Deployments`

## Suggested Description

```
A production-grade self-healing Kubernetes platform I built from scratch in 30 days.

Features:
✅ Go microservice with Prometheus metrics + Jaeger distributed tracing
✅ Custom Go Kubernetes controller that auto-heals crash-looping pods
✅ GitHub Actions CI/CD with automated health checks + Helm rollback
✅ Chaos engineering — random pod killing with zero downtime
✅ Canary deployments with replica-ratio and Nginx exact-weight splitting
✅ PodDisruptionBudget for safe node maintenance
✅ Full unit test coverage with fake Kubernetes client

Code: https://github.com/abhay1999/self-healing-k8s-platform

Timestamps:
0:00 - Introduction
1:00 - Running the app
2:00 - Pod crash & self-healing
3:00 - Custom Go controller
4:00 - Observability (Grafana + Jaeger)
5:30 - Bad deploy + auto-rollback
7:00 - Chaos testing
8:00 - Canary deployment
9:30 - Unit tests
10:00 - Wrap-up
```

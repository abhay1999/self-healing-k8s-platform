#!/bin/bash
# kill-pods.sh — Chaos Engineering Script
#
# Kills a random app pod every N seconds to prove the platform self-heals.
# Run this while watching Grafana to demonstrate zero visible downtime.
#
# Usage:
#   ./chaos-testing/kill-pods.sh              # default: every 30 seconds
#   ./chaos-testing/kill-pods.sh 10 app-a     # every 10 seconds, label app=app-a

INTERVAL="${1:-30}"
APP_LABEL="${2:-app-a}"

echo "=== Chaos Test Started ==="
echo "Killing a random pod with label app=$APP_LABEL every ${INTERVAL}s"
echo "Press Ctrl+C to stop."
echo ""

KILL_COUNT=0

while true; do
  POD=$(kubectl get pods -l "app=$APP_LABEL" -o name 2>/dev/null | shuf -n 1)

  if [ -z "$POD" ]; then
    echo "[$(date +%H:%M:%S)] No pods found with label app=$APP_LABEL. Waiting..."
  else
    echo "[$(date +%H:%M:%S)] Chaos kill #$((++KILL_COUNT)): deleting $POD"
    kubectl delete "$POD" --grace-period=0 --force 2>/dev/null
    echo "[$(date +%H:%M:%S)] Deleted. K8s should reschedule a new pod automatically."
    echo ""
    echo "Current pods:"
    kubectl get pods -l "app=$APP_LABEL"
    echo ""
  fi

  sleep "$INTERVAL"
done

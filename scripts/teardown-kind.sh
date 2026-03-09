#!/usr/bin/env bash
# teardown-kind.sh — Delete the optimizer-test kind cluster.
# Safe to run even if the cluster does not exist.
set -euo pipefail

CLUSTER_NAME="optimizer-test"

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "[INFO] Deleting kind cluster '${CLUSTER_NAME}'..."
  kind delete cluster --name "$CLUSTER_NAME"
  echo "[INFO] Cluster deleted."
else
  echo "[INFO] Cluster '${CLUSTER_NAME}' not found — nothing to do."
fi

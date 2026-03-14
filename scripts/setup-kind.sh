#!/usr/bin/env bash
# setup-kind.sh — Spin up a kind cluster and deploy the optimizer controller.
#
# Usage:
#   ./scripts/setup-kind.sh [--skip-build]
#
# Flags:
#   --skip-build   Skip docker build (assumes image already exists locally)
#
# What this script does:
#   1. Creates a kind cluster named "optimizer-test"
#   2. Clones (or updates) the main repo beside this one
#   3. Applies CRDs and RBAC from the main repo
#   4. Builds the controller Docker image
#   5. Loads the image into kind (no registry needed)
#   6. Deploys the controller via the manifests in deploy/
#   7. Waits up to 60 s for the controller to be Ready
#
# Prerequisites:
#   - kind   >= 0.23  (https://kind.sigs.k8s.io/)
#   - kubectl
#   - docker
#   - go 1.26.1+   (for the build step)

set -euo pipefail

# ── configuration ────────────────────────────────────────────────────────────
CLUSTER_NAME="optimizer-test"
MAIN_REPO_URL="https://github.com/k8s-resource-optimizer/intelligent-cluster-optimizer"
MAIN_REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)/../intelligent-cluster-optimizer"
IMAGE_NAME="intelligent-cluster-optimizer:latest"
CONTROLLER_NS="intelligent-optimizer-system"
CONTROLLER_DEPLOY="intelligent-optimizer-controller"
SKIP_BUILD="${1:-}"

# ── helpers ───────────────────────────────────────────────────────────────────
info()  { echo "[INFO]  $*"; }
warn()  { echo "[WARN]  $*" >&2; }
die()   { echo "[ERROR] $*" >&2; exit 1; }

require_tool() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' not found — please install it first."
}

# ── pre-flight ────────────────────────────────────────────────────────────────
require_tool kind
require_tool kubectl
require_tool docker

info "Using kind cluster: $CLUSTER_NAME"
info "Main repo path:     $MAIN_REPO_DIR"

# ── 1. Create (or reuse) kind cluster ─────────────────────────────────────────
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  info "kind cluster '$CLUSTER_NAME' already exists — skipping create"
else
  info "Creating kind cluster '$CLUSTER_NAME'..."
  cat <<EOF | kind create cluster --name "$CLUSTER_NAME" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${CLUSTER_NAME}
nodes:
  # One control-plane node
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      - containerPort: 30080
        hostPort: 30080
        protocol: TCP
  # Two worker nodes that simulate a small production cluster
  - role: worker
  - role: worker
EOF
  info "kind cluster created."
fi

# Export kubeconfig so subsequent kubectl and go test commands use this cluster.
KUBECONFIG_FILE="/tmp/kind-${CLUSTER_NAME}.yaml"
kind export kubeconfig --name "$CLUSTER_NAME" --kubeconfig "$KUBECONFIG_FILE"
export KUBECONFIG="$KUBECONFIG_FILE"
info "KUBECONFIG set to $KUBECONFIG_FILE"

# ── 2. Ensure main repo is available ──────────────────────────────────────────
if [ ! -d "$MAIN_REPO_DIR" ]; then
  info "Cloning main repo from $MAIN_REPO_URL ..."
  git clone "$MAIN_REPO_URL" "$MAIN_REPO_DIR"
else
  info "Main repo found at $MAIN_REPO_DIR"
fi

# ── 3. Apply CRDs and RBAC ────────────────────────────────────────────────────
info "Applying CRD..."
kubectl apply -f "${MAIN_REPO_DIR}/config/crd/optimizerconfig-crd.yaml"

info "Creating controller namespace (if needed)..."
kubectl create namespace "$CONTROLLER_NS" --dry-run=client -o yaml | kubectl apply -f -

info "Applying RBAC manifests..."
kubectl apply -f "${MAIN_REPO_DIR}/deploy/serviceaccount.yaml"
kubectl apply -f "${MAIN_REPO_DIR}/deploy/rbac.yaml"

# ── 4. Build controller Docker image ─────────────────────────────────────────
if [ "$SKIP_BUILD" = "--skip-build" ]; then
  info "Skipping image build (--skip-build specified)"
else
  info "Building controller image: $IMAGE_NAME ..."
  docker build \
    --file "${MAIN_REPO_DIR}/Dockerfile" \
    --tag  "$IMAGE_NAME" \
    "$MAIN_REPO_DIR"
  info "Image built successfully."
fi

# ── 5. Load image into kind ───────────────────────────────────────────────────
info "Loading image into kind cluster '$CLUSTER_NAME'..."
kind load docker-image "$IMAGE_NAME" --name "$CLUSTER_NAME"
info "Image loaded."

# ── 6. Deploy the controller ──────────────────────────────────────────────────
info "Deploying controller manifests..."
kubectl apply -f "${MAIN_REPO_DIR}/deploy/service.yaml"
kubectl apply -f "${MAIN_REPO_DIR}/deploy/deployment.yaml"

# ── 7. Wait for the controller to become Ready ────────────────────────────────
info "Waiting up to 60 s for controller to become ready..."
kubectl rollout status deployment/"$CONTROLLER_DEPLOY" \
  --namespace "$CONTROLLER_NS" \
  --timeout 60s

info "Controller is ready."
info ""
info "Run E2E tests with:"
info "  KUBECONFIG=\$(kind get kubeconfig --name ${CLUSTER_NAME}) go test ./e2e/... -v -timeout 5m"

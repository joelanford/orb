#!/usr/bin/env bash

set -euo pipefail

# ─── orb demo ────────────────────────────────────────────────────────────────
#
# Prerequisites:
#   - Go 1.25+ (to build orb)
#   - kind (https://kind.sigs.k8s.io/)
#   - kubectl
#
# This script demonstrates:
#   1. Building orb from source
#   2. Adding an OLM catalog
#   3. Listing configured catalogs
#   4. Searching for packages
#   5. Viewing package info
#   6. Resolving available versions
#   7. Installing the latest version into a kind cluster
# ─────────────────────────────────────────────────────────────────────────────

cd "$(dirname "${BASH_SOURCE[0]}")"
CLUSTER_NAME="orb-demo"
NAMESPACE="operators"
PACKAGE="argocd-operator"

# Colors for section headers
bold=$(tput bold)
reset=$(tput sgr0)

wait_for_spacebar() {
    echo "${bold}(press spacebar to continue)${reset}"
    while true; do
        IFS= read -rsn1 key < /dev/tty
        if [ "$key" = " " ]; then
            break
        fi
    done
}

banner() {
    echo ""
    echo "${bold}=== $1 ===${reset}"
    echo ""
    wait_for_spacebar
}

run() {
    # Print the command, run it, then add a blank line after output
    echo "\$ $*"
    "$@"
    echo ""
}

# ─── Build orb ───────────────────────────────────────────────────────────────

banner "Step 1: Build orb"
run make build

# ─── Clean slate: remove any existing catalog with this name ─────────────────

./orb catalog remove operatorhubio 2>/dev/null || true

# ─── Add a catalog ──────────────────────────────────────────────────────────

banner "Step 2: Add the OperatorHub.io community catalog"
run ./orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest --priority -1000

# ─── Show DB size ──────────────────────────────────────────────────────────

banner "Step 3: Show catalog DB size"
run ls -lh ~/Library/Application\ Support/orb/orb.db

# ─── List catalogs ──────────────────────────────────────────────────────────

banner "Step 4: List configured catalogs"
run ./orb catalog list

# ─── Search for packages ────────────────────────────────────────────────────

banner "Step 5: Search for 'argocd' packages"
run ./orb catalog search argocd

# ─── Package info ───────────────────────────────────────────────────────────

banner "Step 6: View package info for ${PACKAGE}"
run ./orb catalog info "$PACKAGE"

# ─── Resolve versions ───────────────────────────────────────────────────────

banner "Step 7: Resolve available versions for ${PACKAGE}"
run ./orb catalog resolve "$PACKAGE"

# ─── Get the latest bundle image ────────────────────────────────────────────

banner "Step 8: Get the latest bundle image reference"
BUNDLE_IMAGE=$(./orb catalog resolve "$PACKAGE" -o jsonpath='{.items[0].image}')
echo "Latest bundle image: $BUNDLE_IMAGE"
echo ""

# ─── Create a kind cluster ──────────────────────────────────────────────────

banner "Step 9: Create a kind cluster"
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' already exists, deleting it first..."
    run kind delete cluster --name "$CLUSTER_NAME"
fi
run kind create cluster --name "$CLUSTER_NAME" --wait 60s

# ─── Install cert-manager ──────────────────────────────────────────────────

banner "Step 10: Install cert-manager"
run kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
echo "Waiting for cert-manager to be ready..."
run kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s

# ─── Install the operator ───────────────────────────────────────────────────

banner "Step 11: Install ${PACKAGE} into the cluster"
echo "Converting bundle to plain manifests and applying to namespace '${NAMESPACE}'..."
echo ""
run kubectl create namespace "$NAMESPACE"
echo "\$ ./orb bundle convert plain docker://${BUNDLE_IMAGE} -n ${NAMESPACE} --cert-provider cert-manager | kubectl create -f -"
./orb bundle convert plain "docker://${BUNDLE_IMAGE}" -n "$NAMESPACE" --cert-provider cert-manager | kubectl create -f -
echo ""

# ─── Verify ─────────────────────────────────────────────────────────────────

banner "Step 12: Verify the installation"
echo "Waiting for deployment rollout..."
echo ""

# Give the deployment a moment to be created
sleep 5

# Find and wait for deployments in the namespace
DEPLOYMENTS=$(kubectl get deployments -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)
if [ -n "$DEPLOYMENTS" ]; then
    for dep in $DEPLOYMENTS; do
        echo "Waiting for deployment/${dep}..."
        kubectl rollout status deployment/"$dep" -n "$NAMESPACE" --timeout=120s || true
    done
fi

echo ""
run kubectl get all -n "$NAMESPACE"

# ─── Create an ArgoCD instance ───────────────────────────────────────────────

banner "Step 13: Create an ArgoCD instance"
kubectl create -f - <<EOF
apiVersion: argoproj.io/v1beta1
kind: ArgoCD
metadata:
  name: example-argocd
  namespace: ${NAMESPACE}
spec: {}
EOF
echo ""

# ─── Wait for ArgoCD and port-forward ────────────────────────────────────────

banner "Step 14: Wait for ArgoCD server and port-forward"
run kubectl rollout status deployment/example-argocd-server -n "$NAMESPACE" --timeout=120s
ADMIN_PASSWORD=$(kubectl get secret example-argocd-cluster -n "$NAMESPACE" -o jsonpath='{.data.admin\.password}' | base64 -d)
echo "ArgoCD UI available at http://localhost:8080"
echo "  username: admin"
echo "  password: ${ADMIN_PASSWORD}"
echo ""
run kubectl port-forward -n "$NAMESPACE" deployment/example-argocd-server 8080:8080

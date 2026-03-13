#!/usr/bin/env bash

set -euo pipefail

# ─── orb demo ────────────────────────────────────────────────────────────────
#
# Prerequisites:
#   - Go 1.25+ (to build orb)
#   - kind (https://kind.sigs.k8s.io/)
#   - kubectl
#   - helm
#
# This script demonstrates:
#   1. Building and installing orb
#   2. Adding and querying an OLM catalog
#   3. Installing an operator with plain manifests (bundle convert + kubectl)
#   4. Installing and upgrading an operator with the orb-getter Helm plugin
# ─────────────────────────────────────────────────────────────────────────────

cd "$(dirname "${BASH_SOURCE[0]}")"
INTERACTIVE="${INTERACTIVE:-false}"
SKIP_PLAIN="${SKIP_PLAIN:-false}"
SKIP_HELM="${SKIP_HELM:-false}"

# Use temporary directories so the demo starts clean
# and doesn't affect the user's real orb or helm config.
DEMO_TMPDIR=$(mktemp -d)
export ORB_DATA_DIR="$DEMO_TMPDIR/orb"
export HELM_DATA_HOME="$DEMO_TMPDIR/helm/data"
export HELM_CONFIG_HOME="$DEMO_TMPDIR/helm/config"
export HELM_CACHE_HOME="$DEMO_TMPDIR/helm/cache"

CLUSTER_NAME="orb-demo-$(date +%s)"
cleanup() {
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    rm -rf "$DEMO_TMPDIR"
}
trap cleanup EXIT
NS_PLAIN="operators-plain"
NS_HELM="operators-helm"
PACKAGE="keycloak-operator"

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
    if [ "$INTERACTIVE" = "true" ]; then
        wait_for_spacebar
    else
        sleep 1
    fi
}

run() {
    # Print the command, run it, then add a blank line after output.
    # A single argument is passed through eval to support pipes.
    echo "\$ $*"
    if [ $# -eq 1 ]; then
        eval "$1"
    else
        "$@"
    fi
    echo ""
}

wait_for_deployments() {
    local ns=$1
    sleep 5
    DEPLOYMENTS=$(kubectl get deployments -n "$ns" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)
    if [ -n "$DEPLOYMENTS" ]; then
        for dep in $DEPLOYMENTS; do
            echo "Waiting for deployment/${dep}..."
            kubectl rollout status deployment/"$dep" -n "$ns" --timeout=120s || true
        done
    fi
    echo ""
}

# ─── Build and install orb ──────────────────────────────────────────────────

banner "Step 1: Build orb"
run make build
export PATH="${PWD}:${PATH}"

# ─── Add a catalog ──────────────────────────────────────────────────────────

banner "Step 2: Add the OperatorHub.io community catalog"
run orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest --priority -1000

# ─── Show DB size ───────────────────────────────────────────────────────────

banner "Step 3: Show catalog DB size"
run ls -lh "$ORB_DATA_DIR/orb.db"

# ─── List catalogs ──────────────────────────────────────────────────────────

banner "Step 4: List configured catalogs"
run orb catalog list

# ─── Search for packages ────────────────────────────────────────────────────

banner "Step 5: Search for 'keycloak' packages"
run orb catalog search keycloak

# ─── Package info ───────────────────────────────────────────────────────────

banner "Step 6: View package info for ${PACKAGE}"
run orb catalog info "$PACKAGE"

# ─── Resolve versions ───────────────────────────────────────────────────────

banner "Step 7: Resolve available versions for ${PACKAGE}"
run orb catalog resolve "$PACKAGE"

# ─── Get the bundle image for version 26.5.3 ──────────────────────────────────

banner "Step 8: Get the bundle image reference for version 26.5.3"
run orb catalog resolve "$PACKAGE" --version 26.5.3 -o jsonpath='{.items[0].image}'
BUNDLE_IMAGE=$(orb catalog resolve "$PACKAGE" --version 26.5.3 -o jsonpath='{.items[0].image}')
echo ""

# ─── Create a kind cluster ──────────────────────────────────────────────────

banner "Step 9: Create a kind cluster"
run kind create cluster --name "$CLUSTER_NAME" --wait 60s

# ═══════════════════════════════════════════════════════════════════════════
#  Plain Manifests Demo
# ═══════════════════════════════════════════════════════════════════════════

if [ "$SKIP_PLAIN" != "true" ]; then

# ─── Install with plain manifests ────────────────────────────────────────────

banner "Step 11: Install ${PACKAGE} with plain manifests (bundle convert + kubectl)"
run kubectl create namespace "$NS_PLAIN"
PLAIN_CONFIG="${DEMO_TMPDIR}/plain-config.yaml"
echo "watchNamespace: ${NS_PLAIN}" > "$PLAIN_CONFIG"
run "orb bundle convert plain docker://${BUNDLE_IMAGE} -n ${NS_PLAIN} -c ${PLAIN_CONFIG} | kubectl create -f -"

# ─── Verify plain install ───────────────────────────────────────────────────

banner "Step 12: Verify the plain manifests installation"
echo "Waiting for deployments to roll out..."
echo ""
wait_for_deployments "$NS_PLAIN"
run kubectl get all -n "$NS_PLAIN"

# ─── Uninstall plain manifests ───────────────────────────────────────────────

banner "Step 13: Uninstall plain manifests"
run "orb bundle convert plain docker://${BUNDLE_IMAGE} -n ${NS_PLAIN} -c ${PLAIN_CONFIG} | kubectl delete -f -"

fi

# ═══════════════════════════════════════════════════════════════════════════
#  Helm Plugin Demo
# ═══════════════════════════════════════════════════════════════════════════

if [ "$SKIP_HELM" != "true" ]; then

# ─── Install the orb-getter Helm plugin ──────────────────────────────────────

banner "Step 14: Install the orb-getter Helm plugin"
run helm plugin list
run orb helm-plugin install orb-getter
run helm plugin list

# ─── helm install at version 26.5.3 ──────────────────────────────────────────

banner "Step 15: helm install ${PACKAGE} at version 26.5.3"
run helm install "$PACKAGE" "orb://${PACKAGE}/?version=26.5.3" \
    --namespace "$NS_HELM" \
    --create-namespace \
    --set "watchNamespace=${NS_HELM}"

# ─── Verify helm install ────────────────────────────────────────────────────

banner "Step 16: Verify the helm installation"
echo "Waiting for deployments to roll out..."
echo ""
wait_for_deployments "$NS_HELM"
run helm list --namespace "$NS_HELM"
run kubectl get all -n "$NS_HELM"

# ─── helm upgrade to latest successor ────────────────────────────────────────

banner "Step 17: helm upgrade ${PACKAGE} (no version constraint — latest successor of currently installed version)"
run helm upgrade "$PACKAGE" "orb://${PACKAGE}/" \
    --namespace "$NS_HELM" \
    --create-namespace \
    --set "watchNamespace=${NS_HELM}"

# ─── Verify helm upgrade ────────────────────────────────────────────────────

banner "Step 18: Verify the helm upgrade"
echo "Waiting for deployments to roll out..."
echo ""
wait_for_deployments "$NS_HELM"
run helm list --namespace "$NS_HELM"
run kubectl get all -n "$NS_HELM"

# ─── Uninstall helm release ─────────────────────────────────────────────────

banner "Step 19: Uninstall helm release"
run helm uninstall "$PACKAGE" --namespace "$NS_HELM"

fi

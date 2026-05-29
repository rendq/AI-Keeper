#!/usr/bin/env bash
# Tear down the AIP local kind cluster + local registry container.
#
# Idempotent: missing resources are silently ignored. Pair with
# `hack/kind/kind-with-registry.sh` (or `make kind-up`).
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-aip-dev}"
REGISTRY_NAME="${REGISTRY_NAME:-kind-registry}"

if command -v kind >/dev/null 2>&1; then
  if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
    echo "→ deleting kind cluster '${CLUSTER_NAME}'"
    kind delete cluster --name "${CLUSTER_NAME}"
  else
    echo "  (no kind cluster '${CLUSTER_NAME}' — skipping)"
  fi
else
  echo "  (kind not installed — skipping cluster delete)"
fi

if command -v docker >/dev/null 2>&1; then
  if docker inspect "${REGISTRY_NAME}" >/dev/null 2>&1; then
    echo "→ removing registry container '${REGISTRY_NAME}'"
    docker rm -f "${REGISTRY_NAME}" >/dev/null
  else
    echo "  (no registry container '${REGISTRY_NAME}' — skipping)"
  fi
else
  echo "  (docker not installed — skipping registry delete)"
fi

echo "✅ kind teardown complete"

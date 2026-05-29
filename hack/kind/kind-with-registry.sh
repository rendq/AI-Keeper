#!/usr/bin/env bash
# Bring up a kind cluster wired to a host-side local container registry.
#
# Adapted from the canonical kind upstream pattern:
#   https://kind.sigs.k8s.io/docs/user/local-registry/
#
# After this script succeeds:
#   • a `kind-registry` container is exposed on `localhost:5001`
#   • the kind cluster's containerd uses it as a mirror for `localhost:5001`
#   • a ConfigMap `local-registry-hosting` advertises it to in-cluster tooling
#
# Usage:
#   ./hack/kind/kind-with-registry.sh                 # bring everything up
#   CLUSTER_NAME=foo REGISTRY_PORT=5002 ...           # override defaults
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-aip-dev}"
REGISTRY_NAME="${REGISTRY_NAME:-kind-registry}"
REGISTRY_PORT="${REGISTRY_PORT:-5001}"
REGISTRY_IMAGE="${REGISTRY_IMAGE:-registry:2}"
KIND_CONFIG="${KIND_CONFIG:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/kind-cluster.yaml}"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "✗ required tool '$1' not found in PATH" >&2
    return 1
  }
}

require docker
require kind
require kubectl

# 1) Start (or reuse) a host-local registry container.
if [ "$(docker inspect -f '{{.State.Running}}' "${REGISTRY_NAME}" 2>/dev/null || true)" != "true" ]; then
  echo "→ starting local registry container '${REGISTRY_NAME}' on localhost:${REGISTRY_PORT}"
  docker run \
    -d --restart=always \
    -p "127.0.0.1:${REGISTRY_PORT}:5000" \
    --network bridge \
    --name "${REGISTRY_NAME}" \
    "${REGISTRY_IMAGE}"
else
  echo "→ reusing already-running registry container '${REGISTRY_NAME}'"
fi

# 2) Create the kind cluster (idempotent — skip if it already exists).
if kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  echo "→ kind cluster '${CLUSTER_NAME}' already exists, skipping create"
else
  echo "→ creating kind cluster '${CLUSTER_NAME}'"
  kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}"
fi

# 3) Wire each node's containerd to the host registry via a registry-mirror
#    config drop-in. Port 5000 is the in-network registry port; localhost:5001
#    is what the host sees. Both names work because of step 4.
REGISTRY_DIR="/etc/containerd/certs.d/localhost:${REGISTRY_PORT}"
for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
  docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
  cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
[host."http://${REGISTRY_NAME}:5000"]
EOF
done

# 4) Connect the registry to the kind network so nodes can reach it by name.
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REGISTRY_NAME}")" = 'null' ]; then
  docker network connect "kind" "${REGISTRY_NAME}"
fi

# 5) Document the registry via a well-known ConfigMap.
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

echo "✅ kind cluster '${CLUSTER_NAME}' + registry localhost:${REGISTRY_PORT} ready"

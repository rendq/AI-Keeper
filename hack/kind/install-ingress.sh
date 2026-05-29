#!/usr/bin/env bash
# Install the kind-flavoured ingress-nginx controller into the active kind
# cluster. The kind-cluster.yaml above labels the control-plane node with
# `ingress-ready=true` and maps host ports 80/443 to it, so this manifest is
# all that's needed to terminate ingress on `localhost`.
#
# Pinned to ingress-nginx v1.11.2 — refresh deliberately when bumping kind.
set -euo pipefail

INGRESS_MANIFEST="${INGRESS_MANIFEST:-https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.2/deploy/static/provider/kind/deploy.yaml}"

command -v kubectl >/dev/null 2>&1 || {
  echo "✗ kubectl not found in PATH" >&2
  exit 1
}

echo "→ applying ingress-nginx manifest"
kubectl apply -f "${INGRESS_MANIFEST}"

echo "→ waiting for ingress-nginx controller to become ready (≤180s)"
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

echo "✅ ingress-nginx ready on localhost:80 / localhost:443"

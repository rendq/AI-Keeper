#!/usr/bin/env bash
# Run kubeconform on any rendered CRD bases / Helm templates.
# Used by .pre-commit-config.yaml and `make lint-k8s`.
set -euo pipefail
shopt -s nullglob globstar

files=(
  config/crd/bases/*.yaml
  deploy/helm/aip/templates/**/*.yaml
)

if [ ${#files[@]} -eq 0 ]; then
  exit 0
fi

exec kubeconform -strict -summary -ignore-missing-schemas "${files[@]}"

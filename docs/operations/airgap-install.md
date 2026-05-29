# Air-gapped Installation Guide

## Overview

AIP supports fully air-gapped deployment where no external network access is available. All container images, Helm charts, and skill packs are pre-bundled.

## Prerequisites

- Internal container registry (Harbor, registry:2, or equivalent)
- Internal Helm chart repository (ChartMuseum or Harbor)
- `aikctl` binary available on a machine with internet access (for packing)

## Step 1: Pack Artifacts (Internet-connected Machine)

```bash
# Generate offline bundle with all images + charts
aikctl airgap pack \
  --version 2.0.0 \
  --output /tmp/aip-airgap-bundle.tar.gz \
  --include-marketplace-packs "healthcare,finance"

# Bundle contents:
# - All AIP container images (amd64 + arm64)
# - Helm chart archive
# - Selected marketplace skill packs
# - SHA256 checksums
```

## Step 2: Transfer Bundle

Transfer `aip-airgap-bundle.tar.gz` to the air-gapped environment via approved media (USB, DVD, SFTP jump host).

## Step 3: Load into Internal Registry

```bash
# On air-gapped machine with registry access
aikctl airgap load \
  --bundle /path/to/aip-airgap-bundle.tar.gz \
  --registry registry.internal.example.com:5000

# Verify images loaded
aikctl airgap verify --registry registry.internal.example.com:5000
```

## Step 4: Deploy with Airgap Values

```yaml
# values-airgap.yaml
global:
  imageRegistry: registry.internal.example.com:5000

airgap:
  enabled: true
  mirrorRegistry: "registry.internal.example.com:5000"
  skipConnectivityCheck: true
  ntpServer: "ntp.internal.example.com"

marketplace:
  enabled: true
  registry:
    endpoint: "registry.internal.example.com:5000"
    insecure: false
```

```bash
helm install aip deploy/helm/ai-keeper \
  -f deploy/helm/ai-keeper/values-p2.yaml \
  -f values-airgap.yaml \
  -n aik-system --create-namespace
```

## Step 5: Verify Installation

```bash
aikctl status --airgap
# Checks: all pods running, no ImagePullBackOff, no external DNS lookups
```

## Updating in Air-gapped Environment

```bash
# On internet-connected machine
aikctl airgap pack --version 2.1.0 --output update-bundle.tar.gz --delta-from 2.0.0

# Transfer and load
aikctl airgap load --bundle update-bundle.tar.gz --registry registry.internal.example.com:5000

# Upgrade
helm upgrade aip deploy/helm/ai-keeper \
  -f deploy/helm/ai-keeper/values-p2.yaml \
  -f values-airgap.yaml \
  --set global.imageTag=2.1.0
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| ImagePullBackOff | Missing image in mirror | Re-run `aikctl airgap load` |
| CrashLoopBackOff on federation | External DNS lookup | Ensure `federation.enabled: false` or configure internal peers |
| Marketplace empty | Packs not loaded | Run `aikctl airgap load` with `--include-marketplace-packs` |

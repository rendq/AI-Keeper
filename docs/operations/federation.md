# Federation Setup Guide

## Overview

Federation enables multiple AI-Keeper clusters to share resources (marketplace listings, policies) while maintaining independent operation. Two modes are supported:

- **Mesh**: All clusters peer directly (best for ≤5 clusters)
- **Hub-spoke**: Central hub coordinates, spokes connect only to hub (best for large deployments)

## Mesh Mode Setup

### 1. Enable on Each Cluster

```yaml
# Cluster A
federation:
  enabled: true
  mode: "mesh"
  cluster:
    name: cluster-a
    region: us-east-1
  peers:
    - name: cluster-b
      endpoint: https://cluster-b.example.com:8443
      caSecretRef: cluster-b-ca
    - name: cluster-c
      endpoint: https://cluster-c.example.com:8443
      caSecretRef: cluster-c-ca
```

### 2. Initialize PKI

```bash
# Generate federation CA (do once)
aikctl federation init-ca --output federation-ca/

# Issue per-cluster certs
aikctl federation issue-cert \
  --ca federation-ca/ \
  --cluster cluster-a \
  --sans "fed.cluster-a.example.com" \
  --output certs/cluster-a/
```

### 3. Distribute CA Secrets

```bash
# On cluster-a, create secrets for each peer's CA
kubectl create secret generic cluster-b-ca \
  --from-file=ca.crt=federation-ca/ca.crt -n aik-system
```

### 4. Deploy

```bash
helm upgrade ai-keeper deploy/helm/ai-keeper \
  -f deploy/helm/ai-keeper/values-p2.yaml \
  -f values-federation.yaml \
  -n aik-system
```

## Hub-Spoke Mode Setup

```yaml
# Hub cluster
federation:
  enabled: true
  mode: "hub-spoke"
  cluster:
    name: hub
    region: us-east-1
  # Hub has no peers listed — spokes connect to it

# Spoke cluster
federation:
  enabled: true
  mode: "hub-spoke"
  cluster:
    name: spoke-eu
    region: eu-west-1
  peers:
    - name: hub
      endpoint: https://fed.hub.example.com:8443
      caSecretRef: hub-ca
```

## Resource Sync Configuration

```yaml
federation:
  sync:
    intervalSeconds: 30
    resources:
      - skillistings     # Marketplace content
      - policies         # Cedar/OPA policies
      # NOT synced by default: audit events, tenant data
```

## Conflict Resolution

When the same resource is modified in multiple clusters:

1. **Last-writer-wins** (default) — highest timestamp wins
2. **Manual** — conflicts are flagged for admin review

```yaml
federation:
  sync:
    conflictResolution: "last-writer-wins"  # or "manual"
```

## Operational Commands

```bash
# Check federation status
aikctl federation status

# Force immediate sync
aikctl federation sync --now

# List pending conflicts (manual mode)
aikctl federation conflicts list

# Resolve a conflict
aikctl federation conflicts resolve <id> --keep local|remote
```

## Network Requirements

| Port | Protocol | Direction | Purpose |
|------|----------|-----------|---------|
| 8443 | gRPC/TLS | Bidirectional (mesh) or spoke→hub | Resource sync |
| 9090 | HTTP | Internal only | Metrics |

## Troubleshooting

| Issue | Diagnosis | Resolution |
|-------|-----------|------------|
| Peer unreachable | `aikctl federation status` shows timeout | Check firewall rules for port 8443 |
| Sync lag growing | Check `aik_federation_sync_lag_seconds` metric | Increase `sync.intervalSeconds` or check network |
| Policy conflicts | `aikctl federation conflicts list` | Resolve manually or switch to last-writer-wins |

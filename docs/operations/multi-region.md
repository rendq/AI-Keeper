# Multi-Region Configuration

## Prerequisites

- AIP P2 deployed in each region
- Network connectivity between clusters (port 8443 for federation)
- TLS certificates for inter-cluster communication

## Architecture

```
Region A (us-east-1)          Region B (eu-west-1)
┌──────────────────┐          ┌──────────────────┐
│  AIP Cluster     │◄────────►│  AIP Cluster     │
│  + Federation    │  TLS/gRPC│  + Federation    │
│  + Cedar (local) │          │  + Cedar (local) │
└──────────────────┘          └──────────────────┘
```

## Setup Steps

### 1. Enable Federation in Each Cluster

```yaml
# values-region-a.yaml
federation:
  enabled: true
  cluster:
    name: us-east-1
    region: us-east-1
  peers:
    - name: eu-west-1
      endpoint: https://fed.eu-west-1.example.com:8443
      caSecretRef: eu-west-1-ca
```

### 2. Create TLS Secrets

```bash
# Generate CA and certs per cluster
aikctl federation init-pki --cluster us-east-1 --output certs/
kubectl create secret tls federation-tls \
  --cert=certs/us-east-1.crt --key=certs/us-east-1.key -n aik-system
```

### 3. Configure Sync Resources

```yaml
federation:
  sync:
    intervalSeconds: 30
    resources:
      - skillistings    # Share marketplace listings
      - policies        # Propagate Cedar policies
```

### 4. Verify Connectivity

```bash
aikctl federation status
# Expected: all peers show "Connected"
```

## Data Residency

- **Policy sync**: Cedar policies replicate globally (stateless evaluation)
- **Audit data**: Stays in originating region (no cross-region replication)
- **Marketplace listings**: Metadata syncs globally; artifacts pulled from nearest registry

## Failover Behavior

- Federation operates in eventual-consistency mode
- If a peer is unreachable, local cluster continues with last-known state
- Reconnection triggers incremental sync (not full resync)

## Monitoring

```bash
# Check federation metrics
kubectl port-forward svc/aip-federation 9090:9090 -n aik-system
curl http://localhost:9090/metrics | grep aip_federation_
```

Key metrics:
- `aip_federation_sync_lag_seconds` — replication delay per peer
- `aip_federation_peer_status` — 1=connected, 0=disconnected
- `aip_federation_conflicts_total` — policy merge conflicts

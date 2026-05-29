# AIP Phase 2 Architecture

## Overview

P2 extends the AIP platform with enterprise-grade capabilities: a Skill Marketplace, multi-cluster Federation, Cedar-based fine-grained authorization, air-gapped deployment support, and automated compliance reporting.

```
┌─────────────────────────────────────────────────────────────┐
│                      AIP Control Plane                       │
├──────────┬──────────┬───────────┬───────────┬──────────────┤
│ Manager  │ Gateway  │ PDP/Cedar │  Audit    │ Compliance   │
│          │          │  Engine   │  Sink     │ Report Engine│
├──────────┴──────────┴───────────┴───────────┴──────────────┤
│                     Federation Layer                         │
│  ┌─────────┐   ┌─────────┐   ┌─────────┐                  │
│  │Cluster A│◄─►│Cluster B│◄─►│Cluster C│  (mesh/hub-spoke)│
│  └─────────┘   └─────────┘   └─────────┘                  │
├─────────────────────────────────────────────────────────────┤
│                     Marketplace                              │
│  ┌──────────┐  ┌───────────┐  ┌──────────────┐            │
│  │ Registry │  │ Review Svc│  │ Pack Merge   │            │
│  │ (OCI)    │  │           │  │ Engine       │            │
│  └──────────┘  └───────────┘  └──────────────┘            │
├─────────────────────────────────────────────────────────────┤
│                     Data Plane                               │
│  Model Router │ Runtime │ Channels │ Storage │ Audit Store  │
└─────────────────────────────────────────────────────────────┘
```

## New Components (P2)

| Component | Purpose | CRD |
|-----------|---------|-----|
| Marketplace Service | Skill pack publishing, discovery, installation | `SkillListing` |
| Pack Merge Engine | Merges industry skill packs into tenant config | — |
| Federation Controller | Cross-cluster resource sync and policy propagation | `FederationPeer` |
| Cedar Engine | Fine-grained authorization (replaces OPA PDP) | — |
| Compliance Report Engine | Automated compliance evidence generation | `ComplianceReport` |
| Holds API | Legal/compliance hold on data deletion | `Hold` |
| Airgap Packer | Offline artifact bundling (`aikctl airgap pack`) | — |

## Key Design Decisions

1. **Cedar over OPA** — Cedar provides strongly-typed, analyzable policies with formal verification support. Deployed as a sidecar or standalone service; OPA PDP remains as fallback.

2. **Federation Mesh** — Peer-to-peer gossip for resource metadata sync. Hub-spoke mode available for strict network topologies.

3. **Marketplace OCI Registry** — Skill packs are OCI artifacts. Reuses existing container registry infrastructure and tooling.

4. **Compliance as Code** — Report definitions are CRDs. Evidence collection is automated via audit log queries and policy evaluation snapshots.

## Deployment Modes

| Mode | Federation | Airgap | Cedar | Marketplace |
|------|-----------|--------|-------|-------------|
| Standard | ✗ | ✗ | ✗ | ✓ |
| Enterprise | ✓ | ✗ | ✓ | ✓ |
| Air-gapped | ✗ | ✓ | ✓ | ✓ (local) |
| Multi-Region | ✓ | ✗ | ✓ | ✓ (federated) |

## Related Docs

- [Multi-Region Operations](operations/multi-region.md)
- [Air-gapped Installation](operations/airgap-install.md)
- [Cedar Engine Guide](operations/cedar-engine.md)
- [Federation Setup](operations/federation.md)

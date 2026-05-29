# Cedar Policy Engine Guide

## Overview

Cedar is an optional replacement for the OPA-based PDP. It provides strongly-typed policies with formal verification support, enabling provable authorization guarantees.

## Enabling Cedar

```yaml
# In your values override
cedar:
  enabled: true
  engine:
    replicas: 2
  policyStore:
    backend: configmap  # configmap | s3 | git
```

When `cedar.enabled: true`, the Cedar engine replaces OPA PDP for authorization decisions. The OPA PDP remains deployed as fallback (configurable).

## Policy Structure

Cedar policies follow the `principal, action, resource` model:

```cedar
// Allow tenant admins to manage agents
permit(
  principal in AIK::Role::"tenant-admin",
  action in [AIK::Action::"CreateAgent", AIK::Action::"DeleteAgent"],
  resource in AIK::Tenant::"acme-corp"
);

// Deny cross-tenant data access
forbid(
  principal,
  action in [AIK::Action::"ReadData"],
  resource
) unless {
  principal.tenant == resource.tenant
};
```

## Policy Store Backends

### ConfigMap (default, dev/test)

```bash
kubectl create configmap cedar-policies \
  --from-file=policies/ -n aik-system
```

### S3 (production)

```yaml
cedar:
  policyStore:
    backend: s3
    s3:
      bucket: "my-cedar-policies"
      prefix: "policies/"
```

### Git (GitOps workflow)

```yaml
cedar:
  policyStore:
    backend: git
    git:
      repo: "https://git.internal/aip-policies.git"
      branch: "main"
      path: "cedar/"
```

## Migration from OPA

1. Enable Cedar alongside OPA (shadow mode):
   ```yaml
   cedar:
     enabled: true
   pdp:
     enabled: true  # Keep OPA running
   ```

2. Compare decisions:
   ```bash
   aikctl policy audit --compare cedar,opa --duration 24h
   ```

3. Once satisfied, disable OPA:
   ```yaml
   pdp:
     enabled: false
   ```

## Validation & Testing

```bash
# Validate policy syntax
aikctl cedar validate --path policies/

# Dry-run authorization check
aikctl cedar check \
  --principal "AIK::User::\"alice\"" \
  --action "AIK::Action::\"CreateAgent\"" \
  --resource "AIK::Tenant::\"acme-corp\""
```

## Monitoring

Key metrics (Prometheus):
- `aip_cedar_decision_duration_seconds` — evaluation latency
- `aip_cedar_decisions_total{result="allow|deny"}` — decision counts
- `aip_cedar_policy_load_errors_total` — policy reload failures

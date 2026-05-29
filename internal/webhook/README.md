# `internal/webhook` — AIP ValidatingAdmissionWebhook

Implements task **2.3** (Validating AdmissionWebhook) from
`.kiro/specs/ai-platform/tasks.md`.

## Scope

- **Defence-in-depth validators** for every AIP Kind that needs admission
  guardrails: `Skill`, `Tool`, `Agent`, `Policy`, `AuditEvent`.
- **Cross-field invariants** the OpenAPI schema cannot express:
  - `Tool.spec.governance.sideEffects=destructive` ⇒
    `governance.requiresApproval=true` is mandatory.
  - `Tool.spec.authentication.mode=oauth2_obo` ⇒ `tokenExchangeRef` is
    required.
  - `AuditEvent` CREATE/UPDATE/DELETE — restricted to ServiceAccounts
    annotated `ai-keeper.io/system=true` (Requirement A1.5). Cluster admins
    in the `system:masters` group keep a hatch for manual rescue.
- **Re-validation of regex inputs** (Requirements A2.1—A2.6) so
  in-cluster mutators that bypass admission still produce identical
  errors to the API-server's rejection.

## Wiring

`webhook.SetupWithManager(mgr)` registers each `CustomValidator` with
the controller-runtime Manager's webhook server. Each handler is
exposed under the canonical kubebuilder URL pattern
`/validate-<group>-<version>-<kind>`. The umbrella Helm chart's
`manager` sub-chart (templates under
`deploy/helm/ai-keeper/charts/manager/templates/webhook-*.yaml`) ships:

- `Issuer` (cert-manager self-signed).
- `Certificate` writing the `aip-manager-webhook-tls` Secret.
- `Service` `aip-manager-webhook` (port 443 → containerPort 9443).
- `ValidatingWebhookConfiguration` with one entry per Kind. The CA
  bundle is injected by cert-manager's `cert-manager.io/inject-ca-from`
  annotation.

## Verification

### Unit tests (sandbox-friendly)

```sh
go test ./internal/webhook/... -count=1 -race
make webhook-test  # equivalent
```

### Helm rendering

```sh
make manifests
make helm-validate
.local-tools/bin/helm template aik-test deploy/helm/ai-keeper \
  --set manager.enabled=true | yq 'select(.kind != null) | .kind + "/" + .metadata.name'
```

When `manager.enabled=true` (and the default `manager.webhook.enabled=true`),
helm template renders four resources: `Issuer`, `Certificate`, `Service`,
`ValidatingWebhookConfiguration`.

### kind-cluster end-to-end (requires live cluster + cert-manager)

```sh
make kind-up                         # one-time bring-up
helm upgrade --install aip deploy/helm/ai-keeper \
  --namespace aik-system --create-namespace \
  --set manager.enabled=true
# Expected: HTTP 422
kubectl --context=kind apply -f internal/webhook/testdata/admission/invalid_skill.yaml
# Expected: success
kubectl --context=kind apply -f internal/webhook/testdata/admission/valid_skill.yaml
# Expected: HTTP 422 — non-system SA cannot write AuditEvent
kubectl --context=kind apply -f internal/webhook/testdata/admission/auditevent_user_attempt.yaml
```

The kind-cluster verification depends on a real `kind` cluster +
`cert-manager` (>= v1.13) + the manager Deployment, which lands in
task 3.6. Until then, the unit tests in this package and the helm
template render are the gating checks.

## Files

| Path                                                               | Purpose                                                                               |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------- |
| `doc.go`                                                           | Package overview + scope.                                                             |
| `scheme.go`                                                        | `NewScheme()` builds a `runtime.Scheme` covering every AIP API group.                 |
| `validators.go`                                                    | Shared field validators (`validateResourceRef`, `validateDuration`, …).               |
| `system_sa.go`                                                     | `SystemSAChecker` — the `ai-keeper.io/system=true` SA annotation guard for AuditEvent.      |
| `skill_validator.go` / `tool_validator.go` / `agent_validator.go`  | Kind-specific `CustomValidator` implementations.                                      |
| `policy_validator.go` / `auditevent_validator.go`                  | Kind-specific `CustomValidator` implementations.                                      |
| `setup.go`                                                         | `SetupWithManager(mgr)` — wires every validator into a controller-runtime Manager.    |
| `*_test.go`                                                        | Unit tests covering positive + negative paths for each Kind.                          |
| `testdata/admission/*.yaml`                                        | Hand-written CR fixtures used by the kind-cluster verification commands above.        |

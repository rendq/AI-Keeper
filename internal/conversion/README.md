# `internal/conversion` — AIP CRD ConversionWebhook

> Package: `github.com/ai-keeper/ai-keeper/internal/conversion`
> Task: **2.4** — *ConversionWebhook 占位（仅 v1alpha1）*
> Validates: Requirements A11.1, A11.2 (placeholder)
> Design: design.md §5.4 (three principles), §11.2 (lossy annotation)

## P0 scope (this task)

P0 ships **only** `v1alpha1` for every Kind — every CRD is marked
`+kubebuilder:storageversion` and there is no second served version
yet. As a result the API server has no real conversion to perform,
but every CRD that ever wants `spec.conversion.strategy=Webhook` must
expose a stable `/convert` endpoint **before** P1 flips the strategy
flag. This package fills that contract today as an *echo identity*:

- `Handler.Convert` reads an `apiextensions.k8s.io/v1.ConversionReview`
  request and returns its `request.objects[]` byte-for-byte unchanged
  with `result.status="Success"`. The response `uid` mirrors the
  request `uid` per the apiextensions wire spec.
- Mismatched `desiredAPIVersion` values (e.g. `skill.ai-keeper.io/v1beta1`,
  reserved for P1) return `result.status="Failure"` with a clear
  *“not yet supported”* message — silent passthrough would mask P1
  wiring mistakes.
- A nil `request` returns Failure (never panics) so probe traffic and
  malformed payloads cannot crash the manager pod.
- `ServeHTTP` rejects non-`POST` methods with 405; otherwise it JSON
  decodes the body, calls `Convert`, and writes the response back.
- `WriteLossyAnnotation(obj, info)` appends an entry to the
  `ai-keeper.io/conversion-lossy` annotation, joining repeated calls with
  ` | `. It is a no-op for `nil` objects or whitespace-only `info`.
  P1 conversion handlers will call this whenever a field has to be
  dropped, defaulted, or re-mapped — providing the audit trail
  Property 7 (design.md §12) round-trip checks.

## Wiring

`SetupWithManager(mgr)` registers the handler at `/convert` on the
controller-runtime Manager's webhook server (default port 9443):

```go
import conversionwiring "github.com/ai-keeper/ai-keeper/internal/conversion"
// ...
if err := conversionwiring.SetupWithManager(mgr); err != nil {
    return err
}
```

`cmd/manager/main.go::setupWebhooks` already calls this alongside the
ValidatingAdmissionWebhook setup from task 2.3, so a single
`make manager` brings both routes online once task 3.6 lands the full
manager bootstrap.

## Helm wiring

Unlike Validating / Mutating webhooks, Kubernetes has **no**
`ConversionWebhookConfiguration` resource — the webhook config lives
*inside* each CRD's `spec.conversion`. P0 keeps every CRD on
`spec.conversion.strategy=None` (kubebuilder default) and ships
`deploy/helm/ai-keeper/charts/manager/templates/webhook-conversion-config.yaml`
as a comment-only placeholder documenting the P1 wiring:

```yaml
spec:
  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: ["v1"]
      clientConfig:
        service:
          name: aip-manager-webhook
          namespace: aik-system
          path: /convert
          port: 443
        # CA bundle injected via the cert-manager.io/inject-ca-from
        # annotation on the CRD itself.
```

The `aip-manager-webhook` Service and the cert-manager Issuer +
Certificate from task 2.3 cover both routes — no extra serving infra
is needed when P1 flips the flag.

## Tests

| Test | Asserts |
|---|---|
| `TestEchoIdentity` | Three random AIP CRs (Skill / Agent / Policy) round-trip byte-identical with `desiredAPIVersion=skill.ai-keeper.io/v1alpha1`; UID is echoed; status=Success. |
| `TestUnknownTargetReturnsFailed` | `desiredAPIVersion=skill.ai-keeper.io/v1beta1` returns `Failure` with a message that names `v1beta1` and *"not yet supported"*. |
| `TestNilRequest` | Empty `ConversionReview{}` returns Failure without panic. |
| `TestServeHTTP` | The handler is mountable under `httptest.Server`; full JSON round-trip succeeds at `/convert`. |
| `TestServeHTTPMethodNotAllowed` | Non-POST methods return 405. |
| `TestWriteLossyAnnotation` | Repeated calls merge with ` | `; nil/empty inputs are no-ops. |

Run:

```bash
go test ./internal/conversion/... -count=1
go test ./internal/conversion/... -run TestEchoIdentity -count=1   # task 2.4 verification
```

## P1 plan (when v1beta1 lands)

1. Promote each Kind's `groupversion_info.go` to add a `v1beta1`
   package and move `+kubebuilder:storageversion` onto the new version.
2. Add per-Kind hub/spoke conversion functions
   (`Skill.ConvertTo` / `Skill.ConvertFrom` etc.) and replace the
   `Handler` echo branch with a Kind-aware dispatcher; keep the
   `desiredAPIVersion` whitelist as the gate.
3. Whenever a conversion is lossy, call
   `WriteLossyAnnotation(obj, "v1alpha1→v1beta1: <reason>")` so the
   round-trip is auditable.
4. Property 7 (design.md §12) lands as a `gopter`-driven PBT under
   `internal/conversion/` and runs in CI:
   `convert(convert(x, v1beta1), v1alpha1) ≡ x` modulo the lossy
   annotation set.
5. Helm: replace the comment-only
   `webhook-conversion-config.yaml` with the
   `+kubebuilder:webhook:` markers that emit
   `spec.conversion.strategy=Webhook` into each CRD's manifest under
   `config/crd/bases/`. cert-manager `cert-manager.io/inject-ca-from`
   annotations on the CRD then pin the trust anchor without manual CA
   wiring.

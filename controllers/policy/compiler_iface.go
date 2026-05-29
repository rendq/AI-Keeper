package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// Bundle is the output of the policy compiler: an opaque blob that the
// PDP loads to evaluate decisions, plus a monotonic version counter and
// a content hash for drift detection.
//
// Bundle values are immutable; callers must not mutate the underlying
// `Bytes` slice. The default in-process [NoopCompiler] populates
// `Bytes` with the canonical JSON shape so distribution wiring can be
// unit-tested without depending on the OPA toolchain.
type Bundle struct {
	// Version is monotonically increasing within a single
	// PolicyController process for any given namespace. Production
	// compilers MUST persist a monotonic counter so the value survives
	// restarts; the default in-memory implementation derives the next
	// version from the input slice's length and revision counter.
	Version int64

	// Hash is the lower-case hex-encoded SHA-256 of `Bytes`, prefixed
	// with `sha256:`. The reconciler propagates this value into
	// `status.bundleHash` so PDPs can be inspected for drift.
	Hash string

	// Bytes is the bundle payload. The shape is opaque to the
	// reconciler and the PDP client.
	Bytes []byte
}

// CompileOption holds optional inputs to the compiler. Reserved for
// future expansion (e.g. injecting a tenant prefix); kept here so the
// interface signature does not need to change.
type CompileOption struct {
	// PreviousVersion is the latest monotonic version already
	// distributed. Compilers SHOULD return `PreviousVersion + 1` on
	// every successful compile so PDPs can reject downgrade attempts.
	PreviousVersion int64
}

// Compiler turns a slice of validated, conflict-free Policies into an
// opaque PDP bundle. Real implementations land in task 5.1.
type Compiler interface {
	Compile(ctx context.Context, policies []*policyv1alpha1.Policy, opt CompileOption) (Bundle, error)
}

// NoopCompiler is a deterministic in-process [Compiler] used by unit
// tests and dev clusters. It serialises the input slice into a
// canonical JSON shape, hashes it with SHA-256 and returns the result
// alongside `opt.PreviousVersion + 1`.
//
// The "canonical JSON" form is achieved by sorting policies by
// `<namespace>/<name>` and feeding their `Spec` blocks through the
// stdlib JSON encoder, which guarantees stable map key ordering on
// Go ≥ 1.12. The output is deterministic for any input ordering, which
// is exactly what tests and drift correction need.
type NoopCompiler struct{}

// Compile returns a deterministic Bundle for the given slice.
func (NoopCompiler) Compile(_ context.Context, policies []*policyv1alpha1.Policy, opt CompileOption) (Bundle, error) {
	type entry struct {
		Key  string                    `json:"key"`
		Spec policyv1alpha1.PolicySpec `json:"spec"`
	}
	out := make([]entry, 0, len(policies))
	for _, p := range policies {
		if p == nil {
			continue
		}
		key := p.Namespace + "/" + p.Name
		out = append(out, entry{Key: key, Spec: p.Spec})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	raw, err := json.Marshal(out)
	if err != nil {
		return Bundle{}, fmt.Errorf("policy: noop compile marshal: %w", err)
	}
	sum := sha256.Sum256(raw)
	return Bundle{
		Version: opt.PreviousVersion + 1,
		Hash:    "sha256:" + hex.EncodeToString(sum[:]),
		Bytes:   raw,
	}, nil
}

// Compile-time interface assertion.
var _ Compiler = NoopCompiler{}

package conversion

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LossyAnnotationKey is the well-known annotation that records lossy
// fields produced during a CRD conversion. The value is a
// pipe-separated audit trail of `from→to: reason` entries — design.md
// §11.2 / glossary entry "lossy annotation".
const LossyAnnotationKey = "ai-keeper.io/conversion-lossy"

// lossySeparator joins audit entries inside the LossyAnnotationKey
// value. A space-padded pipe is human-readable and survives `kubectl
// describe` formatting without escaping.
const lossySeparator = " | "

// WriteLossyAnnotation merges `info` into the
// `ai-keeper.io/conversion-lossy` annotation on `obj`. Existing entries are
// preserved and joined with " | ". Empty (or whitespace-only) `info`
// is a no-op so callers can pass conversion results unconditionally.
//
// The function is exported because P1 conversion handlers (per-Kind
// alpha↔beta) call it from outside the package, and the round-trip
// PBT (Property 7, design.md §12) asserts the annotation contains
// every lossy transformation.
//
// Validates: Requirements A11.2, A11.3 (lossy audit trail).
func WriteLossyAnnotation(obj client.Object, info string) {
	if obj == nil || strings.TrimSpace(info) == "" {
		return
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	if existing, ok := annotations[LossyAnnotationKey]; ok && existing != "" {
		annotations[LossyAnnotationKey] = existing + lossySeparator + info
	} else {
		annotations[LossyAnnotationKey] = info
	}
	obj.SetAnnotations(annotations)
}

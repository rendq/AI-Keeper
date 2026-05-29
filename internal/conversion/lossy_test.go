package conversion

import (
	"strings"
	"testing"
)

// TestWriteLossyAnnotation asserts repeated calls to
// WriteLossyAnnotation merge into a single pipe-separated audit trail
// rather than overwriting prior entries, and that empty `info` is a
// no-op.
//
// Validates: Requirements A11.2, A11.3 (lossy audit trail).
func TestWriteLossyAnnotation(t *testing.T) {
	t.Parallel()
	obj := minimalSkill()
	const first = "v1alpha1→v1beta1: dropped spec.experimentalKnob"
	const second = "v1alpha1→v1beta1: stability remapped beta→stable"

	WriteLossyAnnotation(obj, first)
	WriteLossyAnnotation(obj, second)

	got, ok := obj.GetAnnotations()[LossyAnnotationKey]
	if !ok {
		t.Fatalf("expected annotation %q to be set", LossyAnnotationKey)
	}
	if !strings.Contains(got, first) {
		t.Fatalf("annotation missing first entry %q: got %q", first, got)
	}
	if !strings.Contains(got, second) {
		t.Fatalf("annotation missing second entry %q: got %q", second, got)
	}
	if !strings.Contains(got, " | ") {
		t.Fatalf("expected entries joined by ' | '; got %q", got)
	}
	// Empty / whitespace-only info is a no-op (callers may pass it
	// unconditionally).
	WriteLossyAnnotation(obj, "")
	WriteLossyAnnotation(obj, "   ")
	if obj.GetAnnotations()[LossyAnnotationKey] != got {
		t.Fatalf("empty info must be a no-op; got %q", obj.GetAnnotations()[LossyAnnotationKey])
	}

	// nil obj must not panic.
	WriteLossyAnnotation(nil, "should be ignored")
}

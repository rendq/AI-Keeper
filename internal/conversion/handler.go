package conversion

import (
	"encoding/json"
	"fmt"
	"net/http"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Handler serves apiextensions.k8s.io/v1 ConversionReview requests.
//
// For P0 (only v1alpha1 served & stored) the handler is an echo
// identity: it returns the input objects unchanged with
// `result.status=Success`. Mismatched targets (e.g. v1beta1, which
// does not exist yet) return `result.status=Failed` with a clear
// "not yet supported" message instead of silently passing — which
// keeps P1 wiring obvious.
//
// The struct is intentionally empty; hooks for v1alpha1↔v1beta1
// per-Kind conversion functions land in P1 as named fields here.
//
// Validates: Requirements A11.1, A11.2 (placeholder).
type Handler struct{}

// NewHandler returns a Handler ready to be mounted at /convert.
func NewHandler() *Handler { return &Handler{} }

// supportedAPIVersions lists every served {group, version} pair the
// echo path treats as identity. P0 ships only v1alpha1.
var supportedAPIVersions = map[string]struct{}{
	"core.ai-keeper.io/v1alpha1":   {},
	"skill.ai-keeper.io/v1alpha1":  {},
	"agent.ai-keeper.io/v1alpha1":  {},
	"policy.ai-keeper.io/v1alpha1": {},
	"data.ai-keeper.io/v1alpha1":   {},
	"model.ai-keeper.io/v1alpha1":  {},
	"audit.ai-keeper.io/v1alpha1":  {},
}

// Convert implements the ConversionReview RPC. The `request` field of
// the input is consumed; the `response` field of the returned review
// is set with the same UID and either the input objects (echo) or a
// Failed status.
func (h *Handler) Convert(review *apiextensionsv1.ConversionReview) *apiextensionsv1.ConversionReview {
	out := &apiextensionsv1.ConversionReview{TypeMeta: review.TypeMeta}
	resp := &apiextensionsv1.ConversionResponse{}
	out.Response = resp

	if review.Request == nil {
		resp.Result = metav1.Status{
			Status:  metav1.StatusFailure,
			Message: "conversion: empty request",
			Reason:  metav1.StatusReasonBadRequest,
		}
		return out
	}
	resp.UID = review.Request.UID

	desired := review.Request.DesiredAPIVersion
	if _, ok := supportedAPIVersions[desired]; !ok {
		resp.Result = metav1.Status{
			Status: metav1.StatusFailure,
			Message: fmt.Sprintf(
				"conversion: desiredAPIVersion %q is not yet supported (P0 only serves v1alpha1; "+
					"v1beta1 conversion will be wired in task P1 — see design.md §5.4)",
				desired,
			),
			Reason: metav1.StatusReasonInvalid,
		}
		return out
	}

	// Echo identity: copy the inbound objects byte-for-byte. The k8s
	// API server requires the response list to have the same length
	// and order as the request list (apiextensionsv1 docstring) — we
	// uphold that even when bytes are unchanged.
	resp.ConvertedObjects = make([]runtime.RawExtension, len(review.Request.Objects))
	for i, obj := range review.Request.Objects {
		resp.ConvertedObjects[i] = runtime.RawExtension{Raw: append([]byte(nil), obj.Raw...)}
	}
	resp.Result = metav1.Status{Status: metav1.StatusSuccess}
	return out
}

// ServeHTTP wraps Convert in the apiextensions.k8s.io/v1 wire format.
// It is the http.Handler controller-runtime mounts at /convert.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer func() { _ = r.Body.Close() }()

	in := &apiextensionsv1.ConversionReview{}
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		http.Error(w, fmt.Sprintf("conversion: decode request: %v", err), http.StatusBadRequest)
		return
	}

	out := h.Convert(in)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		// At this point headers are committed; log via stderr is
		// out-of-scope. Best-effort drop.
		return
	}
}

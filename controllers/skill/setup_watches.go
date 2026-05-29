package skill

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// EnqueueSkillsForAgentBindings is the controller-runtime [handler.MapFunc]
// the Skill controller registers against the Agent informer (Requirement
// A6.4). For every Agent create / update / delete event, the function
// emits a reconcile.Request for each Skill the Agent's
// `spec.skills[].ref` resolves to, allowing the Skill reconciler to
// recompute `status.referencingAgents`.
//
// Notes:
//
//   - The handler ignores non-Agent objects so it can be reused with a
//     [handler.EnqueueRequestsFromMapFunc] wired into a generic
//     manager.
//   - Malformed refs are skipped silently; the Agent controller will
//     surface them via its own `SkillsResolved=False` condition.
//   - Duplicate skill references inside one Agent only emit one
//     reconcile.Request — workqueue coalescing handles it anyway, but
//     deduping at the source keeps logs tidy in tests.
func EnqueueSkillsForAgentBindings() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		agent, ok := obj.(*agentv1alpha1.Agent)
		if !ok || agent == nil {
			return nil
		}
		seen := map[types.NamespacedName]struct{}{}
		out := make([]reconcile.Request, 0, len(agent.Spec.Skills))
		for _, binding := range agent.Spec.Skills {
			ns, name, ok := parseSkillRefNamespacedName(binding.Ref, agent.Namespace)
			if !ok {
				continue
			}
			key := types.NamespacedName{Namespace: ns, Name: name}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, reconcile.Request{NamespacedName: key})
		}
		return out
	}
}

// parseSkillRefNamespacedName decodes a `skill://<ns>/<name>[@<version>]`
// ResourceRef into its (namespace, name) components, defaulting the
// namespace to `defaultNS` when the path has no `/`. Returns ok=false
// for non-skill refs, malformed refs, or empty names. Mirrors the
// parser used by the agent controller's resolver so the two controllers
// agree on which Skill an Agent binding addresses.
func parseSkillRefNamespacedName(ref sharedv1alpha1.ResourceRef, defaultNS string) (string, string, bool) {
	scheme, path, _, err := ref.Parse()
	if err != nil {
		return "", "", false
	}
	if scheme != sharedv1alpha1.SchemeSkill {
		return "", "", false
	}
	var ns, name string
	if i := strings.IndexByte(path, '/'); i >= 0 {
		ns = path[:i]
		name = path[i+1:]
	} else {
		ns = defaultNS
		name = path
	}
	if ns == "" {
		ns = "default"
	}
	if name == "" {
		return "", "", false
	}
	return ns, name, true
}

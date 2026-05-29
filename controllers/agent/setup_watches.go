package agent

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// EnqueueAgentsForSkill maps a Skill change to a reconcile.Request for
// every Agent in the Skill's namespace whose `spec.skills[].ref`
// addresses that Skill (Requirements A6.1, A6.2). The closure captures
// `c` so the handler can list Agents on demand without holding the
// controller-runtime cache directly.
//
// The handler ignores non-Skill objects so it stays usable inside a
// generic watcher.
func EnqueueAgentsForSkill(c client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		skill, ok := obj.(*skillv1alpha1.Skill)
		if !ok || skill == nil {
			return nil
		}
		list := &agentv1alpha1.AgentList{}
		if err := c.List(ctx, list, client.InNamespace(skill.Namespace)); err != nil {
			// Best-effort: a list error here only delays reconciliation.
			// The controller-runtime workqueue will revisit the Skill
			// on its next informer event, and Agents stay correct
			// because their own steady-state requeue keeps them
			// converging.
			return nil
		}
		seen := map[types.NamespacedName]struct{}{}
		out := make([]reconcile.Request, 0, len(list.Items))
		for i := range list.Items {
			agent := &list.Items[i]
			if !agentBindsSkill(agent, skill) {
				continue
			}
			key := types.NamespacedName{Namespace: agent.Namespace, Name: agent.Name}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, reconcile.Request{NamespacedName: key})
		}
		return out
	}
}

// SkillStatusChangedPredicate returns a controller-runtime predicate
// that fires the Agent watcher only on Skill creates, deletes, or
// status changes (Requirement A6.1 — phase transitions; A6.2 —
// Deprecating flips). Spec-only edits unrelated to `version` do not
// trigger Agent reconciles because the Agent's resolver is keyed on
// Skill `metadata.name` + `spec.version`, not on the spec body itself.
func SkillStatusChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return true },
		DeleteFunc: func(_ event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSkill, ok1 := e.ObjectOld.(*skillv1alpha1.Skill)
			newSkill, ok2 := e.ObjectNew.(*skillv1alpha1.Skill)
			if !ok1 || !ok2 || oldSkill == nil || newSkill == nil {
				// Be conservative on type mismatches: still enqueue.
				return true
			}
			if !equalSkillStatus(oldSkill.Status, newSkill.Status) {
				return true
			}
			if oldSkill.Spec.Version != newSkill.Spec.Version {
				return true
			}
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}

// equalSkillStatus is a cheap structural comparison of the Skill
// status fields the Agent reconciler keys off: `phase`, the SkillReady
// / SkillDeprecating conditions, and the resolved version surface that
// drives `attachedSkills`. We deliberately do NOT use
// reflect.DeepEqual on the whole status struct because timestamps
// inside unrelated conditions (e.g. EvalPassing) would otherwise
// enqueue every Agent on every periodic re-eval.
func equalSkillStatus(a, b skillv1alpha1.SkillStatus) bool {
	if a.Phase != b.Phase {
		return false
	}
	gates := []string{
		skillv1alpha1.SkillReady,
		skillv1alpha1.SkillDeprecating,
	}
	for _, g := range gates {
		ac := findSkillCondition(a.Conditions, g)
		bc := findSkillCondition(b.Conditions, g)
		switch {
		case ac == nil && bc == nil:
			continue
		case ac == nil || bc == nil:
			return false
		case ac.Status != bc.Status:
			return false
		case ac.Reason != bc.Reason:
			return false
		}
	}
	return true
}

// findSkillCondition walks `conds` for the named condition. Local
// helper kept so the watch wiring stays independent of the reconciler
// internals.
func findSkillCondition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

// agentBindsSkill reports whether `agent` references the supplied
// `skill` via any enabled binding. The version suffix is intentionally
// not compared so a re-published Skill version still triggers the
// referring Agents.
func agentBindsSkill(agent *agentv1alpha1.Agent, skill *skillv1alpha1.Skill) bool {
	if agent == nil || skill == nil {
		return false
	}
	for _, binding := range agent.Spec.Skills {
		if binding.Enabled != nil && !*binding.Enabled {
			continue
		}
		ns, name, ok := parseAgentSkillRefNS(binding.Ref, agent.Namespace)
		if !ok {
			continue
		}
		if ns == skill.Namespace && name == skill.Name {
			return true
		}
	}
	return false
}

// parseAgentSkillRefNS decodes a `skill://<ns>/<name>[@<version>]` ref
// into (namespace, name) components, defaulting the namespace to
// `defaultNS` for namespace-less refs. Mirrors the helper in
// controllers/skill/setup_watches.go to keep the two controllers in
// sync on which Skill an Agent binding addresses.
func parseAgentSkillRefNS(ref sharedv1alpha1.ResourceRef, defaultNS string) (string, string, bool) {
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

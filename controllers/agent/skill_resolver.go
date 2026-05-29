package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
	"github.com/ai-keeper/ai-keeper/controllers/skill"
	"github.com/ai-keeper/ai-keeper/internal/resolver"
)

// SkillResolverResult is the outcome of resolving an Agent's
// `spec.skills[]` against the live cluster. It mirrors the structure
// the reconciler writes onto `status.attachedSkills`.
type SkillResolverResult struct {
	// Resolved lists, for each resolved Skill, its canonical
	// `skill://<ns>/<name>@<version>` ResourceRef.
	Resolved []sharedv1alpha1.ResourceRef

	// Missing is the subset of `spec.skills[]` whose ref does not
	// resolve to any candidate Skill in the cluster.
	Missing []sharedv1alpha1.ResourceRef

	// Unsatisfiable is the subset of `spec.skills[]` whose
	// `versionConstraint` excludes every candidate version found in
	// the cluster (Requirement A4.2).
	Unsatisfiable []sharedv1alpha1.ResourceRef

	// UsesDeprecated is true when at least one resolved Skill has
	// `Deprecating=True` (Requirement A4.6 / A6.2).
	UsesDeprecated bool

	// DeprecatedSkills enumerates the resolved refs whose Skill is
	// in the Deprecating phase. Useful for surfacing operator-visible
	// detail in the condition message.
	DeprecatedSkills []sharedv1alpha1.ResourceRef
}

// SkillResolver resolves an Agent's `spec.skills[]` bindings.
//
// Implementations MUST be deterministic so reconciles stay idempotent
// (Requirement F12).
type SkillResolver interface {
	Resolve(ctx context.Context, agent *agentv1alpha1.Agent) (SkillResolverResult, error)
}

// ClusterSkillResolver is the production [SkillResolver]. It walks the
// cluster via the supplied controller-runtime client to materialise
// each binding.
type ClusterSkillResolver struct {
	Client client.Client
}

// NewClusterSkillResolver constructs a ClusterSkillResolver.
func NewClusterSkillResolver(c client.Client) *ClusterSkillResolver {
	return &ClusterSkillResolver{Client: c}
}

// Resolve implements [SkillResolver].
//
// Behaviour:
//
//  1. For each `spec.skills[i]` enabled binding, parse the ref into
//     (namespace, name) and list every Skill in the namespace whose
//     name matches (exact `metadata.name` first, then
//     [resolver.LabelSkillName]).
//  2. Apply the ref's `@version` (if any) and `versionConstraint`
//     using [resolver.Constraint]. Empty constraint = wildcard.
//  3. Pick the highest semver candidate that:
//     - matches the constraint, AND
//     - is not in `experimental` stability (mirrors the Skill resolver
//     to keep production agents from accidentally pulling a
//     beta-or-lower revision).
//  4. Track Deprecating / missing / unsatisfiable as separate result
//     channels so the reconciler can map each to the right Condition.
func (r *ClusterSkillResolver) Resolve(ctx context.Context, agent *agentv1alpha1.Agent) (SkillResolverResult, error) {
	if r == nil {
		return SkillResolverResult{}, errors.New("agent: nil ClusterSkillResolver")
	}
	if agent == nil {
		return SkillResolverResult{}, errors.New("agent: nil receiver")
	}
	out := SkillResolverResult{}
	for _, binding := range agent.Spec.Skills {
		if binding.Enabled != nil && !*binding.Enabled {
			continue
		}
		ns, name, version, err := parseSkillRef(binding.Ref, agent.Namespace)
		if err != nil {
			return SkillResolverResult{}, fmt.Errorf("agent: skill[%s]: %w", binding.Ref, err)
		}
		candidates, err := r.listSkills(ctx, ns, name)
		if err != nil {
			return SkillResolverResult{}, err
		}
		if len(candidates) == 0 {
			out.Missing = append(out.Missing, binding.Ref)
			continue
		}

		// Combine the optional ref-level `@version` and the
		// versionConstraint into one effective constraint expression.
		// The intersection is implicit: a candidate must satisfy
		// `version` (when given) AND `versionConstraint` (when given).
		picked := pickSkill(candidates, version, binding.VersionConstraint)
		if picked == nil {
			out.Unsatisfiable = append(out.Unsatisfiable, binding.Ref)
			continue
		}

		ref, err := skill.SkillResourceRef(picked)
		if err != nil {
			return SkillResolverResult{}, fmt.Errorf("agent: skill[%s]: build resolved ref: %w", binding.Ref, err)
		}
		out.Resolved = append(out.Resolved, ref)
		if common.IsConditionTrue(picked, skillv1alpha1.SkillDeprecating) {
			out.UsesDeprecated = true
			out.DeprecatedSkills = append(out.DeprecatedSkills, ref)
		}
	}
	return out, nil
}

// listSkills returns every Skill in `ns` whose logical name is `name`,
// applying the same conventions as the production resolver: exact
// `metadata.name` match first, then a list-by-label fallback for
// `<name>-<version>` named objects (see [resolver.LabelSkillName]).
func (r *ClusterSkillResolver) listSkills(ctx context.Context, ns, name string) ([]skillv1alpha1.Skill, error) {
	out := []skillv1alpha1.Skill{}
	var single skillv1alpha1.Skill
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &single); {
	case err == nil:
		out = append(out, single)
	case apierrors.IsNotFound(err):
		// continue with label-selector list
	default:
		return nil, fmt.Errorf("agent: get Skill %s/%s: %w", ns, name, err)
	}
	var list skillv1alpha1.SkillList
	sel := labels.SelectorFromSet(labels.Set{resolver.LabelSkillName: name})
	if err := r.Client.List(ctx, &list, &client.ListOptions{Namespace: ns, LabelSelector: sel}); err != nil {
		return nil, fmt.Errorf("agent: list Skill (label %s=%s): %w", resolver.LabelSkillName, name, err)
	}
	for i := range list.Items {
		s := list.Items[i]
		// Skip the exact-name hit we already collected.
		if len(out) > 0 && s.Name == out[0].Name {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// parseSkillRef decodes a `skill://<ns>/<name>[@<version>]` ref into
// its (namespace, name, version) components, defaulting the namespace
// to the supplied `defaultNS` when the path has no `/`.
func parseSkillRef(ref sharedv1alpha1.ResourceRef, defaultNS string) (ns, name, version string, err error) {
	scheme, path, ver, perr := ref.Parse()
	if perr != nil {
		return "", "", "", fmt.Errorf("malformed ref %q: %w", string(ref), perr)
	}
	if scheme != sharedv1alpha1.SchemeSkill {
		return "", "", "", fmt.Errorf("scheme = %q, want %q", scheme, sharedv1alpha1.SchemeSkill)
	}
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
		return "", "", "", fmt.Errorf("empty name in %q", string(ref))
	}
	return ns, name, ver, nil
}

// pickSkill picks the highest-semver candidate that satisfies both
// the optional `version` exact-match and the optional
// `versionConstraint` npm-style range. Experimental candidates are
// excluded so production agents do not accidentally pull a pre-beta
// revision (mirrors `internal/resolver.pickHighestSatisfying`).
func pickSkill(candidates []skillv1alpha1.Skill, version, versionConstraint string) *skillv1alpha1.Skill {
	if len(candidates) == 0 {
		return nil
	}
	var c *resolver.Constraint
	if strings.TrimSpace(versionConstraint) != "" {
		parsed, err := resolver.ParseConstraint(versionConstraint)
		if err != nil {
			return nil
		}
		c = parsed
	}
	var best *skillv1alpha1.Skill
	for i := range candidates {
		cand := &candidates[i]
		if cand.Spec.Stability == sharedv1alpha1.StageExperimental {
			continue
		}
		if version != "" && string(cand.Spec.Version) != version {
			continue
		}
		if c != nil && !c.Match(cand.Spec.Version) {
			continue
		}
		if best == nil || cand.Spec.Version.Compare(best.Spec.Version) > 0 {
			best = cand
		}
	}
	return best
}

// FuncSkillResolver adapts a plain function to [SkillResolver] for
// table-driven unit tests.
type FuncSkillResolver func(ctx context.Context, agent *agentv1alpha1.Agent) (SkillResolverResult, error)

// Resolve delegates to the wrapped function.
func (f FuncSkillResolver) Resolve(ctx context.Context, agent *agentv1alpha1.Agent) (SkillResolverResult, error) {
	return f(ctx, agent)
}

// Compile-time assertions.
var (
	_ SkillResolver = (*ClusterSkillResolver)(nil)
	_ SkillResolver = FuncSkillResolver(nil)
)

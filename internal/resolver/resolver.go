package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/skill"
)

// LabelSkillName groups multiple Skill versions of the same logical
// skill. When the resolver looks up a sub-skill by its base name it
// first tries an exact `metadata.name` match (single-version case);
// failing that it lists every Skill in the namespace carrying this
// label set to the requested base name. This convention is used by
// publishing tooling (aikctl / pack manifests) that mints
// `<name>-<version>` named objects.
const LabelSkillName = "ai-keeper.io/skill-name"

// ErrMalformedRef is returned when the resolver cannot parse a
// ResourceRef supplied via `spec.implementation.requires[*]`. The Skill
// controller surfaces this as a transient error so the user has time
// to fix the ref before the 1-hour TTL expires.
var ErrMalformedRef = errors.New("resolver: malformed reference")

// Resolver resolves `Skill.spec.implementation.requires` against the
// live cluster. It is the production implementation of
// [skill.Resolver] (the interface declared by the Skill controller).
type Resolver struct {
	Client client.Client
}

// NewResolver returns a Resolver that will use the supplied
// controller-runtime client for all cluster lookups. The function does
// not touch the cluster — the manager wires it up at startup.
func NewResolver(c client.Client) *Resolver {
	return &Resolver{Client: c}
}

// Resolve implements [skill.Resolver].
//
// Behaviour:
//
//   - Each entry in `requires.tools`, `requires.dataSources` and
//     `requires.models` is looked up by its [shared.ResourceRef]. Missing
//     objects are added to the `Missing` slice unchanged. Present
//     objects are echoed into `Resolved`.
//   - Each entry in `requires.skills` is resolved via npm-style
//     [Constraint] semantics against every Skill version surfaced by
//     [Resolver.ListSkillsByName]. Stability ≥ beta is required; any
//     experimental candidate is filtered out (Requirement F12). The
//     highest satisfying version wins.
//   - The transitive sub-skill graph is fed through [TopoSort] to
//     surface cycles. When a cycle is detected `Cyclic=true` and the
//     `Resolved` block is left non-nil but ignored by the controller
//     (Requirement A3.6).
//   - Malformed references are reported via the error return so the
//     controller can apply the standard exponential backoff. Missing
//     references — by contrast — are *not* errors; they signal the
//     1-hour TTL path (Requirements A3.4 / A3.5).
func (r *Resolver) Resolve(ctx context.Context, sk *skillv1alpha1.Skill) (skill.ResolveResult, error) {
	if r == nil {
		return skill.ResolveResult{}, errors.New("resolver: nil receiver")
	}
	if sk == nil {
		return skill.ResolveResult{}, errors.New("resolver: nil skill")
	}
	out := skill.ResolveResult{}
	if sk.Spec.Implementation.Requires == nil {
		// No declared dependencies — trivially resolved.
		return out, nil
	}
	req := sk.Spec.Implementation.Requires

	// ---- Models ---------------------------------------------------------
	for _, dep := range req.Models {
		ns, name, _, err := parseRefPath(dep.Ref, sk.Namespace, shared.SchemeModel)
		if err != nil {
			return skill.ResolveResult{}, err
		}
		ok, lookupErr := r.modelExists(ctx, ns, name)
		if lookupErr != nil {
			return skill.ResolveResult{}, lookupErr
		}
		if !ok {
			out.Missing = append(out.Missing, dep.Ref)
			continue
		}
		out.Resolved.Models = append(out.Resolved.Models, skillv1alpha1.SkillResolvedModel{
			Alias:       dep.Alias,
			ResolvedRef: dep.Ref,
		})
	}

	// ---- Tools ----------------------------------------------------------
	for _, dep := range req.Tools {
		ns, name, _, err := parseRefPath(dep.Ref, sk.Namespace, shared.SchemeTool)
		if err != nil {
			return skill.ResolveResult{}, err
		}
		ok, lookupErr := r.toolExists(ctx, ns, name)
		if lookupErr != nil {
			return skill.ResolveResult{}, lookupErr
		}
		if !ok {
			out.Missing = append(out.Missing, dep.Ref)
			continue
		}
		out.Resolved.Tools = append(out.Resolved.Tools, dep.Ref)
	}

	// ---- DataSources / KnowledgeBases ----------------------------------
	for _, dep := range req.DataSources {
		ns, name, _, err := parseRefPath(dep.Ref, sk.Namespace, shared.SchemeData)
		if err != nil {
			return skill.ResolveResult{}, err
		}
		ok, lookupErr := r.dataExists(ctx, ns, name)
		if lookupErr != nil {
			return skill.ResolveResult{}, lookupErr
		}
		if !ok {
			out.Missing = append(out.Missing, dep.Ref)
			continue
		}
		out.Resolved.DataSources = append(out.Resolved.DataSources, dep.Ref)
	}

	// ---- Sub-skills (with version constraint + cycle detection) --------
	resolvedSkills, missingSkills, edges, allNodes, err := r.resolveSkills(ctx, sk)
	if err != nil {
		return skill.ResolveResult{}, err
	}
	out.Resolved.Skills = append(out.Resolved.Skills, resolvedSkills...)
	out.Missing = append(out.Missing, missingSkills...)

	// Topological sort over every Skill we touched while walking the
	// transitive graph. The root is included so a 2-node A→B→A loop
	// is detected even if only B is reachable from A.
	if len(allNodes) > 0 {
		_, cyclic := TopoSort(allNodes, edges)
		if cyclic {
			out.Cyclic = true
		}
	}

	return out, nil
}

// ----- Lookups ----------------------------------------------------------

// modelExists reports whether the named ModelEndpoint or ModelRouter
// is present in the supplied namespace. Either flavour satisfies a
// `model://` reference.
func (r *Resolver) modelExists(ctx context.Context, ns, name string) (bool, error) {
	var ep modelv1alpha1.ModelEndpoint
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &ep); {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		// fall through to ModelRouter lookup
	default:
		return false, fmt.Errorf("resolver: get ModelEndpoint %s/%s: %w", ns, name, err)
	}
	var rt modelv1alpha1.ModelRouter
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &rt); {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("resolver: get ModelRouter %s/%s: %w", ns, name, err)
	}
}

// toolExists reports whether the named Tool is present.
func (r *Resolver) toolExists(ctx context.Context, ns, name string) (bool, error) {
	var t skillv1alpha1.Tool
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &t); {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("resolver: get Tool %s/%s: %w", ns, name, err)
	}
}

// dataExists reports whether the named DataSource or KnowledgeBase is
// present. Both can satisfy a `data://` reference.
func (r *Resolver) dataExists(ctx context.Context, ns, name string) (bool, error) {
	var ds datav1alpha1.DataSource
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &ds); {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		// fall through to KnowledgeBase lookup
	default:
		return false, fmt.Errorf("resolver: get DataSource %s/%s: %w", ns, name, err)
	}
	var kb datav1alpha1.KnowledgeBase
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &kb); {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("resolver: get KnowledgeBase %s/%s: %w", ns, name, err)
	}
}

// ListSkillsByName returns every Skill version in `namespace` whose
// logical name is `name`. The lookup tries an exact `metadata.name`
// match first (single-version case), then falls back to the
// [LabelSkillName] grouping label so callers that mint
// `<name>-<version>` named objects keep working.
func (r *Resolver) ListSkillsByName(ctx context.Context, namespace, name string) ([]skillv1alpha1.Skill, error) {
	if r == nil {
		return nil, errors.New("resolver: nil receiver")
	}
	if name == "" {
		return nil, errors.New("resolver: empty name")
	}
	out := []skillv1alpha1.Skill{}
	// Exact-name match.
	var single skillv1alpha1.Skill
	switch err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &single); {
	case err == nil:
		out = append(out, single)
	case apierrors.IsNotFound(err):
		// keep going; perhaps caller uses the label convention.
	default:
		return nil, fmt.Errorf("resolver: get Skill %s/%s: %w", namespace, name, err)
	}
	// Label-selector match.
	var list skillv1alpha1.SkillList
	sel := labels.SelectorFromSet(labels.Set{LabelSkillName: name})
	if err := r.Client.List(ctx, &list, &client.ListOptions{Namespace: namespace, LabelSelector: sel}); err != nil {
		return nil, fmt.Errorf("resolver: list Skill (label %s=%s): %w", LabelSkillName, name, err)
	}
	for _, s := range list.Items {
		// Skip the exact-name hit we already collected.
		if s.Name == name && len(out) > 0 && out[0].Name == name {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ----- Sub-skill resolution + graph -------------------------------------

// resolveSkills resolves every entry in `requires.skills`, walks each
// resolved Skill recursively to build the transitive sub-skill graph,
// and returns the data the caller needs to feed [TopoSort].
func (r *Resolver) resolveSkills(
	ctx context.Context,
	root *skillv1alpha1.Skill,
) (
	resolved []shared.ResourceRef,
	missing []shared.ResourceRef,
	edges map[Node][]Node,
	allNodes []Node,
	err error,
) {
	edges = map[Node][]Node{}
	visited := map[Node]struct{}{}

	rootNode, rootErr := skillNode(root)
	if rootErr != nil {
		return nil, nil, nil, nil, rootErr
	}
	visited[rootNode] = struct{}{}
	allNodes = append(allNodes, rootNode)

	// Resolve direct sub-skills first; they become the root's edges.
	if root.Spec.Implementation.Requires != nil {
		for _, dep := range root.Spec.Implementation.Requires.Skills {
			ns, name, _, perr := parseRefPath(dep.Ref, root.Namespace, shared.SchemeSkill)
			if perr != nil {
				return nil, nil, nil, nil, perr
			}
			candidates, lerr := r.ListSkillsByName(ctx, ns, name)
			if lerr != nil {
				return nil, nil, nil, nil, lerr
			}
			pick, perr := pickHighestSatisfying(candidates, dep.VersionConstraint)
			if perr != nil {
				return nil, nil, nil, nil, perr
			}
			if pick == nil {
				missing = append(missing, dep.Ref)
				continue
			}
			pickedRef, refErr := skill.SkillResourceRef(pick)
			if refErr != nil {
				return nil, nil, nil, nil, refErr
			}
			resolved = append(resolved, pickedRef)
			child := nodeFromRef(pickedRef)
			edges[rootNode] = append(edges[rootNode], child)
			if _, seen := visited[child]; !seen {
				if walkErr := r.walkSubSkills(ctx, pick, edges, visited, &allNodes); walkErr != nil {
					return nil, nil, nil, nil, walkErr
				}
			}
		}
	}
	return resolved, missing, edges, allNodes, nil
}

// walkSubSkills recursively expands `sk`'s sub-skill graph into
// `edges` and `allNodes`, picking the same constraint resolution rules
// the root used. Nodes that have already been visited are short-cut so
// shared sub-trees (diamonds) do not blow up.
func (r *Resolver) walkSubSkills(
	ctx context.Context,
	sk *skillv1alpha1.Skill,
	edges map[Node][]Node,
	visited map[Node]struct{},
	allNodes *[]Node,
) error {
	node, err := skillNode(sk)
	if err != nil {
		return err
	}
	if _, seen := visited[node]; seen {
		return nil
	}
	visited[node] = struct{}{}
	*allNodes = append(*allNodes, node)

	if sk.Spec.Implementation.Requires == nil {
		return nil
	}
	for _, dep := range sk.Spec.Implementation.Requires.Skills {
		ns, name, _, perr := parseRefPath(dep.Ref, sk.Namespace, shared.SchemeSkill)
		if perr != nil {
			return perr
		}
		candidates, lerr := r.ListSkillsByName(ctx, ns, name)
		if lerr != nil {
			return lerr
		}
		pick, pErr := pickHighestSatisfying(candidates, dep.VersionConstraint)
		if pErr != nil {
			return pErr
		}
		if pick == nil {
			// Missing sub-sub-skill — does not break cycle detection,
			// the parent will surface it via missing[]. We still
			// register a placeholder edge so the topological sort sees
			// the call site.
			edges[node] = append(edges[node], Node(string(dep.Ref)))
			continue
		}
		pickedRef, refErr := skill.SkillResourceRef(pick)
		if refErr != nil {
			return refErr
		}
		child := nodeFromRef(pickedRef)
		edges[node] = append(edges[node], child)
		if _, seen := visited[child]; !seen {
			if err := r.walkSubSkills(ctx, pick, edges, visited, allNodes); err != nil {
				return err
			}
		}
	}
	return nil
}

// pickHighestSatisfying returns the candidate with the highest
// [shared.SemVer] that:
//
//   - matches the supplied npm-style range (when non-empty); and
//   - has stability ≥ beta (i.e., not [shared.StageExperimental]).
//
// An empty `constraintStr` matches every non-experimental candidate
// (npm `*` semantics minus pre-release).
func pickHighestSatisfying(candidates []skillv1alpha1.Skill, constraintStr string) (*skillv1alpha1.Skill, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	var (
		c   *Constraint
		err error
	)
	if strings.TrimSpace(constraintStr) != "" {
		c, err = ParseConstraint(constraintStr)
		if err != nil {
			return nil, err
		}
	}
	var best *skillv1alpha1.Skill
	for i := range candidates {
		cand := &candidates[i]
		if cand.Spec.Stability == shared.StageExperimental {
			continue
		}
		if c != nil && !c.Match(cand.Spec.Version) {
			continue
		}
		if best == nil || cand.Spec.Version.Compare(best.Spec.Version) > 0 {
			best = cand
		}
	}
	return best, nil
}

// ----- Helpers ----------------------------------------------------------

// parseRefPath decodes a [shared.ResourceRef] into (namespace, name,
// version) and confirms the scheme matches `wantScheme`. When the path
// has no `/` separator the supplied default namespace is used.
func parseRefPath(ref shared.ResourceRef, defaultNS string, wantScheme shared.ResourceRefScheme) (ns, name, version string, err error) {
	scheme, path, ver, perr := ref.Parse()
	if perr != nil {
		return "", "", "", fmt.Errorf("%w: %v", ErrMalformedRef, perr)
	}
	if scheme != wantScheme {
		return "", "", "", fmt.Errorf("%w: scheme = %q, want %q", ErrMalformedRef, scheme, wantScheme)
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
		return "", "", "", fmt.Errorf("%w: empty name in %q", ErrMalformedRef, string(ref))
	}
	return ns, name, ver, nil
}

// skillNode builds a stable graph identifier from a Skill object using
// its canonical `skill://` ResourceRef.
func skillNode(sk *skillv1alpha1.Skill) (Node, error) {
	ref, err := skill.SkillResourceRef(sk)
	if err != nil {
		return "", err
	}
	return nodeFromRef(ref), nil
}

// nodeFromRef wraps a ResourceRef in the [Node] alias.
func nodeFromRef(r shared.ResourceRef) Node {
	return Node(string(r))
}

// Compile-time interface assertion: the resolver implements the Skill
// controller's interface contract.
var _ skill.Resolver = (*Resolver)(nil)

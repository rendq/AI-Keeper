// Package resolver implements the Skill dependency resolver used by
// the Skill controller (controllers/skill). It owns three concerns:
//
//   - parsing npm-style version constraint strings (`^1.0.0`,
//     `>=1.0.0 <2.0.0`, …) and matching them against [shared.SemVer]
//     candidates;
//   - looking up dependency objects (Tool / ModelEndpoint / DataSource
//     / sub-Skill) in the cluster via a controller-runtime client;
//   - running a topological sort with Kahn's algorithm over the
//     transitive sub-skill graph to detect cycles and produce a
//     deterministic resolution order.
//
// Validates: Requirements A3.3, A3.4, A3.5, A3.6, F12.
package resolver

import (
	"sort"
)

// Node is the canonical identifier used by the topological sorter. It
// is a plain string alias so callers can use any naming scheme that
// produces stable, unique identifiers (e.g. `skill://ns/name@version`).
type Node string

// TopoSort runs Kahn's algorithm over the supplied DAG. It returns a
// linear order in which each node appears before every node it points
// to — that is, for every edge `u -> v` (meaning `u in nodes && v in
// edges[u]`), `u` appears before `v` in the returned slice.
//
// When the graph contains a cycle the function returns the partial
// order produced before the cycle was detected and `cyclic=true`.
// Callers in the Skill resolver MUST treat `cyclic=true` as a permanent
// failure (Requirement A3.6).
//
// Determinism: ties (multiple nodes with in-degree zero at the same
// step) are broken by lexical order of the [Node] string. The
// `nodes` slice MAY contain duplicates; duplicates are silently
// folded into a single graph vertex. Edges that reference vertices not
// listed in `nodes` are tolerated and contribute to the in-degree of
// their target only when the target is itself in `nodes`. Self-loops
// (`edges[u]` contains `u`) are reported as a cycle.
func TopoSort(nodes []Node, edges map[Node][]Node) (order []Node, cyclic bool) {
	// Deduplicate the node set while preserving determinism via sort.
	seen := make(map[Node]struct{}, len(nodes))
	unique := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		unique = append(unique, n)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })

	// Compute in-degree using only edges that point at known vertices
	// so callers can pass partially-known graphs without creating
	// phantom requirements.
	indeg := make(map[Node]int, len(unique))
	for _, n := range unique {
		indeg[n] = 0
	}
	// Self-loop short-circuit — Kahn's would loop forever otherwise
	// because the self edge keeps the node's in-degree above zero.
	for u, vs := range edges {
		for _, v := range vs {
			if u == v {
				if _, ok := seen[u]; ok {
					return nil, true
				}
			}
		}
	}
	for u, vs := range edges {
		if _, ok := seen[u]; !ok {
			continue
		}
		for _, v := range vs {
			if _, ok := seen[v]; !ok {
				continue
			}
			indeg[v]++
		}
	}

	// Initial frontier: every node with in-degree zero, sorted for
	// determinism.
	frontier := make([]Node, 0)
	for _, n := range unique {
		if indeg[n] == 0 {
			frontier = append(frontier, n)
		}
	}
	sort.Slice(frontier, func(i, j int) bool { return frontier[i] < frontier[j] })

	out := make([]Node, 0, len(unique))
	for len(frontier) > 0 {
		// Pop the lexicographically smallest node.
		u := frontier[0]
		frontier = frontier[1:]
		out = append(out, u)
		// Decrement neighbours; collect the ones that just hit zero.
		newly := make([]Node, 0)
		for _, v := range edges[u] {
			if _, ok := seen[v]; !ok {
				continue
			}
			indeg[v]--
			if indeg[v] == 0 {
				newly = append(newly, v)
			}
		}
		if len(newly) > 0 {
			frontier = append(frontier, newly...)
			sort.Slice(frontier, func(i, j int) bool { return frontier[i] < frontier[j] })
		}
	}
	if len(out) != len(unique) {
		// At least one node still has in-degree > 0 — that node sits
		// in a strongly-connected component (cycle).
		return out, true
	}
	return out, false
}

package resolver

import (
	"reflect"
	"testing"
)

func TestTopoSort_Linear(t *testing.T) {
	t.Parallel()
	// A -> B -> C
	nodes := []Node{"A", "B", "C"}
	edges := map[Node][]Node{
		"A": {"B"},
		"B": {"C"},
	}
	order, cyclic := TopoSort(nodes, edges)
	if cyclic {
		t.Fatalf("unexpected cyclic=true")
	}
	if !reflect.DeepEqual(order, []Node{"A", "B", "C"}) {
		t.Fatalf("order = %v, want [A B C]", order)
	}
}

func TestTopoSort_Branching(t *testing.T) {
	t.Parallel()
	// A -> B, A -> C, B -> D, C -> D (diamond).
	nodes := []Node{"A", "B", "C", "D"}
	edges := map[Node][]Node{
		"A": {"B", "C"},
		"B": {"D"},
		"C": {"D"},
	}
	order, cyclic := TopoSort(nodes, edges)
	if cyclic {
		t.Fatalf("unexpected cyclic=true; order=%v", order)
	}
	// Determinism: tie-break by lexical order. B comes before C in
	// the second batch because both have in-degree zero after A is
	// removed.
	want := []Node{"A", "B", "C", "D"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestTopoSort_Cycle(t *testing.T) {
	t.Parallel()
	// A -> B -> A.
	nodes := []Node{"A", "B"}
	edges := map[Node][]Node{
		"A": {"B"},
		"B": {"A"},
	}
	_, cyclic := TopoSort(nodes, edges)
	if !cyclic {
		t.Fatalf("expected cyclic=true")
	}
}

func TestTopoSort_SelfLoop(t *testing.T) {
	t.Parallel()
	nodes := []Node{"A"}
	edges := map[Node][]Node{
		"A": {"A"},
	}
	_, cyclic := TopoSort(nodes, edges)
	if !cyclic {
		t.Fatalf("expected cyclic=true on self-loop")
	}
}

func TestTopoSort_DisconnectedDeterministic(t *testing.T) {
	t.Parallel()
	// Two disconnected components. The deterministic tie-break on the
	// initial frontier should sort by lexical order.
	nodes := []Node{"X", "A", "Y", "B"}
	edges := map[Node][]Node{
		"A": {"B"},
		"X": {"Y"},
	}
	order, cyclic := TopoSort(nodes, edges)
	if cyclic {
		t.Fatalf("unexpected cyclic=true")
	}
	// After A is popped its successor B has in-degree zero, so the
	// frontier becomes {B, X}; sorted lexically that yields B before X.
	want := []Node{"A", "B", "X", "Y"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestTopoSort_EmptyGraph(t *testing.T) {
	t.Parallel()
	order, cyclic := TopoSort(nil, nil)
	if cyclic {
		t.Fatalf("unexpected cyclic=true")
	}
	if len(order) != 0 {
		t.Fatalf("expected empty order, got %v", order)
	}
}

func TestTopoSort_DuplicateNodes(t *testing.T) {
	t.Parallel()
	nodes := []Node{"A", "B", "A"}
	edges := map[Node][]Node{"A": {"B"}}
	order, cyclic := TopoSort(nodes, edges)
	if cyclic {
		t.Fatalf("unexpected cyclic=true")
	}
	if !reflect.DeepEqual(order, []Node{"A", "B"}) {
		t.Fatalf("order = %v, want [A B]", order)
	}
}

func TestTopoSort_UnknownEdgeTarget(t *testing.T) {
	t.Parallel()
	// Edge points to a vertex not in the node set; must be ignored.
	nodes := []Node{"A"}
	edges := map[Node][]Node{
		"A": {"GHOST"},
	}
	order, cyclic := TopoSort(nodes, edges)
	if cyclic {
		t.Fatalf("unexpected cyclic=true")
	}
	if !reflect.DeepEqual(order, []Node{"A"}) {
		t.Fatalf("order = %v, want [A]", order)
	}
}

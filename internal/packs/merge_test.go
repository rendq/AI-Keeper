package packs

import (
	"testing"
)

func TestMerge_NoConflicts(t *testing.T) {
	// base==ours, theirs changed — take theirs.
	base := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	ours := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	theirs := map[string]interface{}{"replicas": 5, "image": "v1.1"}

	result := ThreeWayMerge(base, ours, theirs)

	if result.HasConflicts {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if result.Merged["replicas"] != 5 {
		t.Errorf("expected replicas=5, got %v", result.Merged["replicas"])
	}
	if result.Merged["image"] != "v1.1" {
		t.Errorf("expected image=v1.1, got %v", result.Merged["image"])
	}
}

func TestMerge_UserChange(t *testing.T) {
	// base==theirs, ours changed — keep ours.
	base := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	ours := map[string]interface{}{"replicas": 10, "image": "v1.0"}
	theirs := map[string]interface{}{"replicas": 3, "image": "v1.0"}

	result := ThreeWayMerge(base, ours, theirs)

	if result.HasConflicts {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if result.Merged["replicas"] != 10 {
		t.Errorf("expected replicas=10, got %v", result.Merged["replicas"])
	}
}

func TestMerge_BothChanged(t *testing.T) {
	// Both ours and theirs differ from base — conflict.
	base := map[string]interface{}{"replicas": 3}
	ours := map[string]interface{}{"replicas": 10}
	theirs := map[string]interface{}{"replicas": 5}

	result := ThreeWayMerge(base, ours, theirs)

	if !result.HasConflicts {
		t.Fatal("expected conflicts")
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	c := result.Conflicts[0]
	if c.Path != "replicas" {
		t.Errorf("expected conflict on 'replicas', got %q", c.Path)
	}
	if c.Base != 3 || c.Ours != 10 || c.Theirs != 5 {
		t.Errorf("unexpected conflict values: base=%v ours=%v theirs=%v", c.Base, c.Ours, c.Theirs)
	}
}

func TestMerge_NewFields(t *testing.T) {
	// Theirs introduces a new field not in base or ours — add it.
	base := map[string]interface{}{"replicas": 3}
	ours := map[string]interface{}{"replicas": 3}
	theirs := map[string]interface{}{"replicas": 3, "timeout": "30s"}

	result := ThreeWayMerge(base, ours, theirs)

	if result.HasConflicts {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if result.Merged["timeout"] != "30s" {
		t.Errorf("expected timeout=30s, got %v", result.Merged["timeout"])
	}
	if result.Merged["replicas"] != 3 {
		t.Errorf("expected replicas=3, got %v", result.Merged["replicas"])
	}
}

func TestMerge_DeletedFields(t *testing.T) {
	// Ours deletes a field that theirs keeps unchanged — user deletion wins.
	base := map[string]interface{}{"replicas": 3, "debug": true}
	ours := map[string]interface{}{"replicas": 3} // deleted "debug"
	theirs := map[string]interface{}{"replicas": 3, "debug": true}

	result := ThreeWayMerge(base, ours, theirs)

	if result.HasConflicts {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if _, exists := result.Merged["debug"]; exists {
		t.Error("expected 'debug' to be deleted from merged result")
	}
}

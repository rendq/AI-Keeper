package merge

import (
	"fmt"
	"testing"
)

func TestThreeWayMerge_CleanMerge_NoChanges(t *testing.T) {
	base := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	ours := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	theirs := map[string]interface{}{"replicas": 3, "image": "v1.0"}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if !result.IsClean() {
		t.Fatalf("expected clean merge, got %d conflicts", len(result.Report.Conflicts))
	}
	assertValue(t, result.Merged, "replicas", 3)
	assertValue(t, result.Merged, "image", "v1.0")
}

func TestThreeWayMerge_CleanMerge_OnlyTheirsChanged(t *testing.T) {
	base := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	ours := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	theirs := map[string]interface{}{"replicas": 5, "image": "v1.1"}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if !result.IsClean() {
		t.Fatalf("expected clean merge, got %d conflicts", len(result.Report.Conflicts))
	}
	assertValue(t, result.Merged, "replicas", 5)
	assertValue(t, result.Merged, "image", "v1.1")
}

func TestThreeWayMerge_CleanMerge_OnlyOursChanged(t *testing.T) {
	base := map[string]interface{}{"replicas": 3, "image": "v1.0"}
	ours := map[string]interface{}{"replicas": 10, "image": "v1.0"}
	theirs := map[string]interface{}{"replicas": 3, "image": "v1.0"}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if !result.IsClean() {
		t.Fatalf("expected clean merge, got conflicts")
	}
	assertValue(t, result.Merged, "replicas", 10)
}

func TestThreeWayMerge_AutoMerge_NonOverlapping(t *testing.T) {
	// Ours changes replicas, theirs changes image — no overlap.
	base := map[string]interface{}{"replicas": 3, "image": "v1.0", "port": 8080}
	ours := map[string]interface{}{"replicas": 10, "image": "v1.0", "port": 8080}
	theirs := map[string]interface{}{"replicas": 3, "image": "v2.0", "port": 8080}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if !result.IsClean() {
		t.Fatalf("expected clean auto-merge, got %d conflicts", len(result.Report.Conflicts))
	}
	assertValue(t, result.Merged, "replicas", 10)
	assertValue(t, result.Merged, "image", "v2.0")
	assertValue(t, result.Merged, "port", 8080)
}

func TestThreeWayMerge_Conflict_BothModifiedSameField(t *testing.T) {
	base := map[string]interface{}{"replicas": 3}
	ours := map[string]interface{}{"replicas": 10}
	theirs := map[string]interface{}{"replicas": 5}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if result.IsClean() {
		t.Fatal("expected conflicts")
	}
	if len(result.Report.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Report.Conflicts))
	}
	c := result.Report.Conflicts[0]
	if c.Path != "replicas" {
		t.Errorf("expected path=replicas, got %q", c.Path)
	}
	if c.Hint != HintManual {
		t.Errorf("expected hint=manual, got %s", c.Hint)
	}
}

func TestThreeWayMerge_Conflict_UserDeletedTheirsModified(t *testing.T) {
	base := map[string]interface{}{"debug": true, "replicas": 3}
	ours := map[string]interface{}{"replicas": 3} // deleted debug
	theirs := map[string]interface{}{"debug": false, "replicas": 3} // modified debug

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if result.IsClean() {
		t.Fatal("expected conflict for user-deleted + theirs-modified")
	}
	found := false
	for _, c := range result.Report.Conflicts {
		if c.Path == "debug" {
			found = true
			if c.Hint != HintAcceptTheirs {
				t.Errorf("expected hint=accept-theirs, got %s", c.Hint)
			}
		}
	}
	if !found {
		t.Error("expected conflict on 'debug' path")
	}
}

func TestThreeWayMerge_Conflict_TheirsDeletedOursModified(t *testing.T) {
	base := map[string]interface{}{"debug": true, "replicas": 3}
	ours := map[string]interface{}{"debug": false, "replicas": 3} // modified debug
	theirs := map[string]interface{}{"replicas": 3}               // deleted debug

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if result.IsClean() {
		t.Fatal("expected conflict for theirs-deleted + ours-modified")
	}
	found := false
	for _, c := range result.Report.Conflicts {
		if c.Path == "debug" {
			found = true
			if c.Hint != HintAcceptOurs {
				t.Errorf("expected hint=accept-ours, got %s", c.Hint)
			}
		}
	}
	if !found {
		t.Error("expected conflict on 'debug' path")
	}
}

func TestThreeWayMerge_AddedResources_NewFieldFromTheirs(t *testing.T) {
	base := map[string]interface{}{"replicas": 3}
	ours := map[string]interface{}{"replicas": 3}
	theirs := map[string]interface{}{"replicas": 3, "timeout": "30s"}

	result := ThreeWayMerge(base, ours, theirs, "Service/api")

	if !result.IsClean() {
		t.Fatalf("expected clean merge, got conflicts")
	}
	assertValue(t, result.Merged, "timeout", "30s")
	assertValue(t, result.Merged, "replicas", 3)
}

func TestThreeWayMerge_UserAddedField(t *testing.T) {
	base := map[string]interface{}{"replicas": 3}
	ours := map[string]interface{}{"replicas": 3, "custom": "mine"}
	theirs := map[string]interface{}{"replicas": 3}

	result := ThreeWayMerge(base, ours, theirs, "Service/api")

	if !result.IsClean() {
		t.Fatalf("expected clean merge, got conflicts")
	}
	assertValue(t, result.Merged, "custom", "mine")
}

func TestThreeWayMerge_UserDeletedUnchangedField(t *testing.T) {
	base := map[string]interface{}{"replicas": 3, "debug": true}
	ours := map[string]interface{}{"replicas": 3} // deleted debug
	theirs := map[string]interface{}{"replicas": 3, "debug": true}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if !result.IsClean() {
		t.Fatalf("expected clean merge (user deletion of unchanged field wins)")
	}
	if _, exists := result.Merged["debug"]; exists {
		t.Error("expected 'debug' to be absent from merged result")
	}
}

func TestThreeWayMerge_NestedMaps(t *testing.T) {
	base := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   "my-app",
			"labels": map[string]interface{}{"app": "v1", "team": "backend"},
		},
	}
	ours := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   "my-app",
			"labels": map[string]interface{}{"app": "v1", "team": "platform"}, // changed team
		},
	}
	theirs := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   "my-app",
			"labels": map[string]interface{}{"app": "v2", "team": "backend"}, // changed app
		},
	}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/my-app")

	if !result.IsClean() {
		t.Fatalf("expected clean auto-merge on nested maps, got %d conflicts", len(result.Report.Conflicts))
	}

	metadata, ok := result.Merged["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected metadata to be a map")
	}
	labels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		t.Fatal("expected labels to be a map")
	}
	if labels["app"] != "v2" {
		t.Errorf("expected app=v2, got %v", labels["app"])
	}
	if labels["team"] != "platform" {
		t.Errorf("expected team=platform, got %v", labels["team"])
	}
}

func TestThreeWayMerge_NestedConflict(t *testing.T) {
	base := map[string]interface{}{
		"spec": map[string]interface{}{"replicas": 3},
	}
	ours := map[string]interface{}{
		"spec": map[string]interface{}{"replicas": 10},
	}
	theirs := map[string]interface{}{
		"spec": map[string]interface{}{"replicas": 5},
	}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if result.IsClean() {
		t.Fatal("expected nested conflict")
	}
	if len(result.Report.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Report.Conflicts))
	}
	c := result.Report.Conflicts[0]
	if c.Path != "spec.replicas" {
		t.Errorf("expected path=spec.replicas, got %q", c.Path)
	}
}

func TestThreeWayMergeResourceSet_AddedResource(t *testing.T) {
	base := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 3},
	}
	ours := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 3},
	}
	theirs := map[string]map[string]interface{}{
		"Deployment/app":    {"replicas": 3},
		"Service/new-svc":   {"port": 8080},
	}

	result := ThreeWayMergeResourceSet(base, ours, theirs)

	if result.HasConflicts() {
		t.Fatal("expected no conflicts")
	}
	if len(result.Added) != 1 || result.Added[0] != "Service/new-svc" {
		t.Errorf("expected Added=[Service/new-svc], got %v", result.Added)
	}
	if _, ok := result.Merged["Service/new-svc"]; !ok {
		t.Error("expected new resource in merged set")
	}
}

func TestThreeWayMergeResourceSet_RemovedResource(t *testing.T) {
	base := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 3},
		"Service/old":    {"port": 80},
	}
	ours := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 3},
		"Service/old":    {"port": 80}, // unchanged
	}
	theirs := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 3},
		// Service/old removed upstream
	}

	result := ThreeWayMergeResourceSet(base, ours, theirs)

	if result.HasConflicts() {
		t.Fatal("expected no conflicts")
	}
	if len(result.Removed) != 1 || result.Removed[0] != "Service/old" {
		t.Errorf("expected Removed=[Service/old], got %v", result.Removed)
	}
	if _, ok := result.Merged["Service/old"]; ok {
		t.Error("removed resource should not be in merged set")
	}
}

func TestThreeWayMergeResourceSet_Conflict(t *testing.T) {
	base := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 3},
	}
	ours := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 10},
	}
	theirs := map[string]map[string]interface{}{
		"Deployment/app": {"replicas": 5},
	}

	result := ThreeWayMergeResourceSet(base, ours, theirs)

	if !result.HasConflicts() {
		t.Fatal("expected conflicts")
	}
	report, ok := result.Conflicts["Deployment/app"]
	if !ok {
		t.Fatal("expected conflict report for Deployment/app")
	}
	if !report.HasConflicts() {
		t.Fatal("report should have conflicts")
	}
}

func TestThreeWayMerge_BothChangedToSameValue(t *testing.T) {
	base := map[string]interface{}{"replicas": 3}
	ours := map[string]interface{}{"replicas": 5}
	theirs := map[string]interface{}{"replicas": 5}

	result := ThreeWayMerge(base, ours, theirs, "Deployment/app")

	if !result.IsClean() {
		t.Fatal("expected clean merge when both changed to same value")
	}
	assertValue(t, result.Merged, "replicas", 5)
}

func TestConflictReport_Summary(t *testing.T) {
	report := &ConflictReport{
		ResourceKey: "Deployment/app",
		Conflicts: []Conflict{
			{Path: "replicas", Hint: HintManual, Reason: "both modified"},
		},
	}

	summary := report.Summary()
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestResourceKey(t *testing.T) {
	key := ResourceKey("Deployment", "my-app")
	if key != "Deployment/my-app" {
		t.Errorf("expected Deployment/my-app, got %q", key)
	}
}

func TestThreeWayMerge_BothAddedDifferentValues(t *testing.T) {
	base := map[string]interface{}{}
	ours := map[string]interface{}{"newfield": "user-val"}
	theirs := map[string]interface{}{"newfield": "upstream-val"}

	result := ThreeWayMerge(base, ours, theirs, "ConfigMap/cfg")

	if result.IsClean() {
		t.Fatal("expected conflict when both add same field with different values")
	}
	if len(result.Report.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Report.Conflicts))
	}
	c := result.Report.Conflicts[0]
	if c.Path != "newfield" {
		t.Errorf("expected path=newfield, got %q", c.Path)
	}
	if c.Hint != HintManual {
		t.Errorf("expected hint=manual, got %s", c.Hint)
	}
}

// assertValue is a test helper to verify merged map values.
func assertValue(t *testing.T, m map[string]interface{}, key string, expected interface{}) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("expected key %q to exist in merged map", key)
		return
	}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", expected) {
		t.Errorf("key %q: expected %v, got %v", key, expected, got)
	}
}

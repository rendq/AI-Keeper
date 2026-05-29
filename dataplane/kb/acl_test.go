package kb

import (
	"testing"
)

func makeResult(text, aclUsers, aclGroups, classification string) RetrievalResult {
	meta := map[string]string{
		"classification": classification,
	}
	if aclUsers != "" {
		meta["acl_users"] = aclUsers
	}
	if aclGroups != "" {
		meta["acl_groups"] = aclGroups
	}
	return RetrievalResult{Text: text, Score: 1.0, Metadata: meta}
}

func TestFilterByACL_UserInACL(t *testing.T) {
	ctx := UserContext{UserID: "alice", Groups: nil, ClassificationLevel: "internal"}
	results := []RetrievalResult{
		makeResult("doc1", "alice,bob", "", "public"),
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 result, got %d", len(filtered))
	}
	if filtered[0].Text != "doc1" {
		t.Errorf("expected doc1, got %s", filtered[0].Text)
	}
}

func TestFilterByACL_UserNotInACL(t *testing.T) {
	ctx := UserContext{UserID: "charlie", Groups: nil, ClassificationLevel: "internal"}
	results := []RetrievalResult{
		makeResult("doc1", "alice,bob", "", "public"),
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 0 {
		t.Fatalf("expected 0 results, got %d", len(filtered))
	}
}

func TestFilterByACL_GroupMembership(t *testing.T) {
	ctx := UserContext{UserID: "charlie", Groups: []string{"engineering", "ops"}, ClassificationLevel: "internal"}
	results := []RetrievalResult{
		makeResult("eng doc", "", "engineering,hr", "public"),
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 result, got %d", len(filtered))
	}
}

func TestFilterByACL_ClassificationDenied(t *testing.T) {
	ctx := UserContext{UserID: "alice", Groups: nil, ClassificationLevel: "internal"}
	results := []RetrievalResult{
		makeResult("secret doc", "alice", "", "confidential"),
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 0 {
		t.Fatalf("expected 0 results, got %d", len(filtered))
	}
}

func TestFilterByACL_ClassificationAllowed(t *testing.T) {
	ctx := UserContext{UserID: "admin", Groups: nil, ClassificationLevel: "secret"}
	results := []RetrievalResult{
		makeResult("pub", "admin", "", "public"),
		makeResult("int", "admin", "", "internal"),
		makeResult("conf", "admin", "", "confidential"),
		makeResult("rest", "admin", "", "restricted"),
		makeResult("sec", "admin", "", "secret"),
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 5 {
		t.Fatalf("expected 5 results, got %d", len(filtered))
	}
}

func TestFilterByACL_EmptyACLDenyByDefault(t *testing.T) {
	ctx := UserContext{UserID: "alice", Groups: []string{"admin"}, ClassificationLevel: "secret"}
	results := []RetrievalResult{
		{Text: "unprotected", Score: 1.0, Metadata: map[string]string{"classification": "public"}},
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 0 {
		t.Fatalf("expected 0 results (secure by default), got %d", len(filtered))
	}
}

func TestFilterByACL_NilMetadataDenied(t *testing.T) {
	ctx := UserContext{UserID: "alice", ClassificationLevel: "secret"}
	results := []RetrievalResult{
		{Text: "no meta", Score: 1.0, Metadata: nil},
	}

	filtered := FilterByACL(results, ctx)

	if len(filtered) != 0 {
		t.Fatalf("expected 0 results (nil metadata), got %d", len(filtered))
	}
}

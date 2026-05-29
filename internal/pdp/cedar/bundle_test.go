package cedar

import (
	"testing"
)

func TestCedarBundle_Build(t *testing.T) {
	builder := NewBundleBuilder()
	builder.AddPolicy(PolicyInput{
		Subject:  "User::alice",
		Action:   "invoke",
		Resource: "Skill::summarize",
		Effect:   "allow",
	})
	builder.AddPolicy(PolicyInput{
		Subject:  "User::bob",
		Action:   "read",
		Resource: "Data::reports",
		Effect:   "allow",
	})

	bundle, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if bundle.PolicyCount != 2 {
		t.Errorf("PolicyCount = %d, want 2", bundle.PolicyCount)
	}
	if bundle.Version != 1 {
		t.Errorf("Version = %d, want 1", bundle.Version)
	}
	if bundle.Policies == "" {
		t.Error("Policies should not be empty")
	}
	if bundle.Hash == "" {
		t.Error("Hash should not be empty")
	}
}

func TestCedarBundle_Hash(t *testing.T) {
	// Build the same bundle twice and verify hash is deterministic.
	build := func() string {
		b := NewBundleBuilder()
		b.AddPolicy(PolicyInput{
			Subject:  "User::alice",
			Action:   "invoke",
			Resource: "Skill::summarize",
			Effect:   "allow",
		})
		bundle, err := b.Build()
		if err != nil {
			t.Fatalf("Build() error: %v", err)
		}
		return bundle.Hash
	}

	hash1 := build()
	hash2 := build()
	if hash1 != hash2 {
		t.Errorf("Hash not deterministic: %q != %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("Hash length = %d, want 64 (sha256 hex)", len(hash1))
	}
}

func TestCedarBundle_Incremental(t *testing.T) {
	builder := NewBundleBuilder()
	builder.AddPolicy(PolicyInput{
		Subject:  "User::alice",
		Action:   "invoke",
		Resource: "Skill::summarize",
		Effect:   "allow",
	})

	bundle1, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Add a policy incrementally
	added := []PolicyInput{{
		Subject:  "User::carol",
		Action:   "read",
		Resource: "Data::logs",
		Effect:   "allow",
	}}

	bundle2, err := builder.BuildIncremental(bundle1, added, nil)
	if err != nil {
		t.Fatalf("BuildIncremental() error: %v", err)
	}

	if bundle2.Version <= bundle1.Version {
		t.Errorf("Version did not increment: %d <= %d", bundle2.Version, bundle1.Version)
	}
	if bundle2.PolicyCount != 2 {
		t.Errorf("PolicyCount = %d, want 2", bundle2.PolicyCount)
	}
	if bundle2.Hash == bundle1.Hash {
		t.Error("Hash should differ after adding a policy")
	}
}

func TestCedarBundle_Empty(t *testing.T) {
	builder := NewBundleBuilder()
	_, err := builder.Build()
	if err == nil {
		t.Fatal("Build() with no policies should return error")
	}
}

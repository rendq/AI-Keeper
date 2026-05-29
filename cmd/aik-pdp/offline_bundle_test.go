package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineBundle_LoadPolicies(t *testing.T) {
	dir := t.TempDir()

	// Write sample .rego files.
	policies := map[string]string{
		"allow.rego": `package aip
default allow = false
allow { input.principal.user_id == "admin" }`,
		"deny.rego": `package aip
deny { input.action.verb == "delete" }`,
		"readme.txt": "not a policy",
	}
	for name, content := range policies {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	compiler := NewOfflineBundleCompiler()
	loaded, err := compiler.LoadPolicies(dir)
	if err != nil {
		t.Fatalf("LoadPolicies: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(loaded))
	}
}

func TestOfflineBundle_CompileBundle(t *testing.T) {
	policies := [][]byte{
		[]byte(`package aip
default allow = false
allow { input.principal.user_id == "admin" }`),
	}

	compiler := NewOfflineBundleCompiler()
	bundle, err := compiler.CompileBundle(policies)
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}

	if bundle.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if bundle.Version == 0 {
		t.Error("expected non-zero version")
	}
	if len(bundle.Data) == 0 {
		t.Error("expected non-empty data")
	}

	// Verify the bundle is a valid tar.gz containing .rego.
	modules, _, err := parseBundleTarGz(bundle.Data)
	if err != nil {
		t.Fatalf("bundle not valid tar.gz: %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("expected 1 module in bundle, got %d", len(modules))
	}
}

func TestOfflineBundle_SaveBundle(t *testing.T) {
	bundle := &Bundle{
		Hash:    "sha256:abc123",
		Version: 1,
		Data:    []byte("test-bundle-data"),
	}

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "output", "bundle.tar.gz")

	compiler := NewOfflineBundleCompiler()
	if err := compiler.SaveBundle(bundle, outputPath); err != nil {
		t.Fatalf("SaveBundle: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "test-bundle-data" {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestOfflineBundle_EndToEnd(t *testing.T) {
	// Setup: create policy dir with .rego files.
	policyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(policyDir, "main.rego"), []byte(`package aip
default allow = false
allow { input.principal.user_id == "admin" }`), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "bundle.tar.gz")

	compiler := NewOfflineBundleCompiler()

	// Load.
	policies, err := compiler.LoadPolicies(policyDir)
	if err != nil {
		t.Fatalf("LoadPolicies: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	// Compile.
	bundle, err := compiler.CompileBundle(policies)
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}

	// Save.
	if err := compiler.SaveBundle(bundle, outputPath); err != nil {
		t.Fatalf("SaveBundle: %v", err)
	}

	// Verify output file exists and is loadable.
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	modules, _, err := parseBundleTarGz(data)
	if err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(modules))
	}
}

func TestOfflineBundle_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	compiler := NewOfflineBundleCompiler()
	policies, err := compiler.LoadPolicies(dir)
	if err != nil {
		t.Fatalf("LoadPolicies should not error on empty dir: %v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("expected 0 policies, got %d", len(policies))
	}

	// CompileBundle with empty slice should fail gracefully.
	_, err = compiler.CompileBundle(policies)
	if err == nil {
		t.Fatal("expected error when compiling empty policy set")
	}
}

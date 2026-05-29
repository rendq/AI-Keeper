package packs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func setupTestPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create pack.yaml
	packYAML := `meta:
  name: industry/finance
  version: "1.0.0"
  description: Finance industry pack
  author: aip-team
  dependencies:
    - core/base@1.0.0
  tags:
    - finance
    - compliance
`
	if err := os.WriteFile(filepath.Join(dir, "pack.yaml"), []byte(packYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create values.yaml
	valuesYAML := `replicas: 3
logLevel: info
`
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(valuesYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create manifests directory with sample files
	manifestDir := filepath.Join(dir, "manifests")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest1 := `apiVersion: v1
kind: ConfigMap
metadata:
  name: finance-config
`
	manifest2 := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: finance-agent
`
	if err := os.WriteFile(filepath.Join(manifestDir, "configmap.yaml"), []byte(manifest1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "deployment.yaml"), []byte(manifest2), 0644); err != nil {
		t.Fatal(err)
	}

	// Create checksums.txt with correct hashes
	hash1 := sha256sum([]byte(manifest1))
	hash2 := sha256sum([]byte(manifest2))
	checksums := hash1 + "  manifests/configmap.yaml\n" + hash2 + "  manifests/deployment.yaml\n"
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(checksums), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestLoadPack(t *testing.T) {
	dir := setupTestPack(t)

	pack, err := LoadPack(dir)
	if err != nil {
		t.Fatalf("LoadPack() error: %v", err)
	}

	// Verify metadata
	if pack.Meta.Name != "industry/finance" {
		t.Errorf("Meta.Name = %q, want %q", pack.Meta.Name, "industry/finance")
	}
	if pack.Meta.Version != "1.0.0" {
		t.Errorf("Meta.Version = %q, want %q", pack.Meta.Version, "1.0.0")
	}
	if pack.Meta.Author != "aip-team" {
		t.Errorf("Meta.Author = %q, want %q", pack.Meta.Author, "aip-team")
	}
	if len(pack.Meta.Dependencies) != 1 || pack.Meta.Dependencies[0] != "core/base@1.0.0" {
		t.Errorf("Meta.Dependencies = %v, want [core/base@1.0.0]", pack.Meta.Dependencies)
	}

	// Verify values loaded
	if pack.Values["replicas"] != 3 {
		t.Errorf("Values[replicas] = %v, want 3", pack.Values["replicas"])
	}

	// Verify manifests discovered
	if len(pack.Manifests) != 2 {
		t.Errorf("len(Manifests) = %d, want 2", len(pack.Manifests))
	}

	// Verify checksums loaded
	if len(pack.Checksums) != 2 {
		t.Errorf("len(Checksums) = %d, want 2", len(pack.Checksums))
	}
}

func TestLoadPack_MissingPackYAML(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadPack(dir)
	if err == nil {
		t.Fatal("LoadPack() expected error for missing pack.yaml, got nil")
	}
}

func TestValidateChecksums(t *testing.T) {
	dir := setupTestPack(t)

	pack, err := LoadPack(dir)
	if err != nil {
		t.Fatalf("LoadPack() error: %v", err)
	}

	// Valid checksums should pass
	if err := ValidateChecksums(pack, dir); err != nil {
		t.Fatalf("ValidateChecksums() unexpected error: %v", err)
	}
}

func TestValidateChecksums_Mismatch(t *testing.T) {
	dir := setupTestPack(t)

	pack, err := LoadPack(dir)
	if err != nil {
		t.Fatalf("LoadPack() error: %v", err)
	}

	// Tamper with a manifest file to cause mismatch
	tampered := filepath.Join(dir, "manifests", "configmap.yaml")
	if err := os.WriteFile(tampered, []byte("tampered content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateChecksums(pack, dir); err == nil {
		t.Fatal("ValidateChecksums() expected error for tampered file, got nil")
	}
}

func TestListManifests(t *testing.T) {
	dir := setupTestPack(t)

	pack, err := LoadPack(dir)
	if err != nil {
		t.Fatalf("LoadPack() error: %v", err)
	}

	paths, err := ListManifests(pack, dir)
	if err != nil {
		t.Fatalf("ListManifests() error: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("len(paths) = %d, want 2", len(paths))
	}

	// All returned paths should exist
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("manifest path %s does not exist: %v", p, err)
		}
	}
}

func TestListManifests_MissingFile(t *testing.T) {
	dir := setupTestPack(t)

	pack, err := LoadPack(dir)
	if err != nil {
		t.Fatalf("LoadPack() error: %v", err)
	}

	// Remove a manifest file
	os.Remove(filepath.Join(dir, "manifests", "configmap.yaml"))

	_, err = ListManifests(pack, dir)
	if err == nil {
		t.Fatal("ListManifests() expected error for missing file, got nil")
	}
}

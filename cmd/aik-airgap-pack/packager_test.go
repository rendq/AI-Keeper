package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// mockDockerClient implements DockerClient for testing.
type mockDockerClient struct {
	saveErr error
}

func (m *mockDockerClient) SaveImages(_ context.Context, images []string, w io.Writer) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	// Write a small payload to simulate docker save output.
	_, err := w.Write([]byte("mock-image-data"))
	return err
}

func TestCollectImages(t *testing.T) {
	p := NewPackager(&mockDockerClient{})
	manifest := p.DefaultManifest()
	images := p.CollectImages(manifest)

	if len(images) != len(ComponentImages) {
		t.Fatalf("expected %d images, got %d", len(ComponentImages), len(images))
	}

	// Verify each image is fully qualified.
	for i, img := range images {
		expected := DefaultRegistry + "/" + ComponentImages[i] + ":" + DefaultTag
		if img != expected {
			t.Errorf("image[%d] = %q, want %q", i, img, expected)
		}
	}
}

func TestDefaultManifest(t *testing.T) {
	p := NewPackager(&mockDockerClient{})
	manifest := p.DefaultManifest()

	// Must include all 7 component images.
	expectedCount := 7
	if len(manifest.Images) != expectedCount {
		t.Fatalf("DefaultManifest has %d images, want %d", len(manifest.Images), expectedCount)
	}

	if manifest.Registry != DefaultRegistry {
		t.Errorf("registry = %q, want %q", manifest.Registry, DefaultRegistry)
	}
	if manifest.Tag != DefaultTag {
		t.Errorf("tag = %q, want %q", manifest.Tag, DefaultTag)
	}
}

func TestCreateBundle(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "bundle.tar.gz")

	p := NewPackager(&mockDockerClient{})
	manifest := p.DefaultManifest()

	config := BundleConfig{
		OutputPath: outputPath,
		Images:     p.CollectImages(manifest),
	}

	result, err := p.CreateBundle(context.Background(), config)
	if err != nil {
		t.Fatalf("CreateBundle failed: %v", err)
	}
	if result != outputPath {
		t.Errorf("result = %q, want %q", result, outputPath)
	}

	// Verify the file exists and has content.
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestCreateBundleWithAssets(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "bundle.tar.gz")

	// Create mock helm chart directory.
	helmDir := filepath.Join(tmpDir, "helm")
	os.MkdirAll(helmDir, 0755)
	os.WriteFile(filepath.Join(helmDir, "Chart.yaml"), []byte("name: aip"), 0644)

	// Create mock CRD directory.
	crdDir := filepath.Join(tmpDir, "crds")
	os.MkdirAll(crdDir, 0755)
	os.WriteFile(filepath.Join(crdDir, "agent.yaml"), []byte("kind: CRD"), 0644)

	// Create mock PDP bundle template directory.
	bundleDir := filepath.Join(tmpDir, "pdp-bundle")
	os.MkdirAll(bundleDir, 0755)
	os.WriteFile(filepath.Join(bundleDir, "default.rego"), []byte("package aip"), 0644)

	p := NewPackager(&mockDockerClient{})
	config := BundleConfig{
		OutputPath:         outputPath,
		Images:             []string{"ghcr.io/aip-io/aip-controller:latest"},
		HelmChartPath:      helmDir,
		CRDPath:            crdDir,
		BundleTemplatePath: bundleDir,
	}

	result, err := p.CreateBundle(context.Background(), config)
	if err != nil {
		t.Fatalf("CreateBundle with assets failed: %v", err)
	}

	info, err := os.Stat(result)
	if err != nil {
		t.Fatalf("output not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestCreateBundleInvalidConfig(t *testing.T) {
	p := NewPackager(&mockDockerClient{})

	tests := []struct {
		name   string
		config BundleConfig
	}{
		{
			name:   "empty output path",
			config: BundleConfig{OutputPath: "", Images: []string{"img:v1"}},
		},
		{
			name:   "no images",
			config: BundleConfig{OutputPath: "/tmp/out.tar.gz", Images: nil},
		},
		{
			name:   "empty images slice",
			config: BundleConfig{OutputPath: "/tmp/out.tar.gz", Images: []string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.CreateBundle(context.Background(), tt.config)
			if err == nil {
				t.Error("expected error for invalid config, got nil")
			}
		})
	}
}

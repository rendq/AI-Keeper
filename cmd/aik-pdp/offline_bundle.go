package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OfflineBundleConfig holds configuration for offline bundle compilation.
type OfflineBundleConfig struct {
	PolicyDir  string
	OutputPath string
}

// Bundle represents a compiled OPA bundle ready for PDP consumption.
type Bundle struct {
	Hash    string
	Version int64
	Data    []byte // tar.gz content
}

// OfflineBundleCompiler compiles Rego policies from local filesystem into an OPA bundle.
type OfflineBundleCompiler struct{}

// NewOfflineBundleCompiler creates a new OfflineBundleCompiler.
func NewOfflineBundleCompiler() *OfflineBundleCompiler {
	return &OfflineBundleCompiler{}
}

// LoadPolicies reads all .rego files from the given directory.
func (c *OfflineBundleCompiler) LoadPolicies(dir string) ([][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read policy dir: %w", err)
	}

	var policies [][]byte
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".rego" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read policy %s: %w", entry.Name(), err)
		}
		policies = append(policies, data)
	}
	return policies, nil
}

// CompileBundle compiles raw Rego policy bytes into an OPA-compatible tar.gz bundle.
func (c *OfflineBundleCompiler) CompileBundle(policies [][]byte) (*Bundle, error) {
	if len(policies) == 0 {
		return nil, fmt.Errorf("no policies to compile")
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for i, policy := range policies {
		name := fmt.Sprintf("policy_%d.rego", i)
		hdr := &tar.Header{
			Name:    name,
			Mode:    0644,
			Size:    int64(len(policy)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("write tar header: %w", err)
		}
		if _, err := tw.Write(policy); err != nil {
			return nil, fmt.Errorf("write tar content: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	data := buf.Bytes()
	h := sha256.Sum256(data)

	return &Bundle{
		Hash:    "sha256:" + hex.EncodeToString(h[:]),
		Version: time.Now().Unix(),
		Data:    data,
	}, nil
}

// SaveBundle writes a compiled bundle to the specified output path.
func (c *OfflineBundleCompiler) SaveBundle(bundle *Bundle, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(outputPath, bundle.Data, 0644); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	return nil
}

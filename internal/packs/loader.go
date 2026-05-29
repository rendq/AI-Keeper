package packs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadPack reads a pack directory and returns the parsed PackManifest.
// The directory must contain at least a pack.yaml file.
func LoadPack(dir string) (*PackManifest, error) {
	packFile := filepath.Join(dir, "pack.yaml")
	data, err := os.ReadFile(packFile)
	if err != nil {
		return nil, fmt.Errorf("packs: failed to read pack.yaml: %w", err)
	}

	var pm PackManifest
	if err := yaml.Unmarshal(data, &pm); err != nil {
		return nil, fmt.Errorf("packs: failed to parse pack.yaml: %w", err)
	}

	// Load values.yaml if present.
	valuesFile := filepath.Join(dir, "values.yaml")
	if vData, err := os.ReadFile(valuesFile); err == nil {
		vals := make(map[string]interface{})
		if err := yaml.Unmarshal(vData, &vals); err != nil {
			return nil, fmt.Errorf("packs: failed to parse values.yaml: %w", err)
		}
		pm.Values = vals
	}

	// Load checksums.txt if present.
	checksumFile := filepath.Join(dir, "checksums.txt")
	if cData, err := os.ReadFile(checksumFile); err == nil {
		pm.Checksums = parseChecksums(string(cData))
	}

	// Discover manifests from manifests/ directory.
	manifestDir := filepath.Join(dir, "manifests")
	if entries, err := os.ReadDir(manifestDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
				pm.Manifests = append(pm.Manifests, filepath.Join("manifests", e.Name()))
			}
		}
	}

	return &pm, nil
}

// ValidateChecksums verifies file integrity by comparing SHA-256 digests
// listed in the pack's Checksums map against actual file content in dir.
func ValidateChecksums(pack *PackManifest, dir string) error {
	for relPath, expected := range pack.Checksums {
		actual, err := hashFile(filepath.Join(dir, relPath))
		if err != nil {
			return fmt.Errorf("packs: checksum validation failed for %s: %w", relPath, err)
		}
		if actual != expected {
			return fmt.Errorf("packs: checksum mismatch for %s: expected %s, got %s", relPath, expected, actual)
		}
	}
	return nil
}

// ListManifests returns the absolute paths of all YAML manifests in the pack directory.
func ListManifests(pack *PackManifest, dir string) ([]string, error) {
	var paths []string
	for _, rel := range pack.Manifests {
		abs := filepath.Join(dir, rel)
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("packs: manifest not found: %s", abs)
		}
		paths = append(paths, abs)
	}
	return paths, nil
}

// parseChecksums parses a checksums.txt file with lines formatted as:
// <hex-digest>  <filepath>
func parseChecksums(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			result[parts[1]] = parts[0]
		}
	}
	return result
}

// hashFile computes the SHA-256 hex digest of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

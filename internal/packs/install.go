package packs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// InstallPack applies the pack's manifests to the target tenant namespace.
// This is currently a stub implementation that validates inputs and reads
// manifest files without performing actual Kubernetes API calls.
func InstallPack(ctx context.Context, pack *PackManifest, tenant string, values map[string]interface{}) error {
	if pack == nil {
		return fmt.Errorf("packs: pack manifest is nil")
	}
	if tenant == "" {
		return fmt.Errorf("packs: tenant must not be empty")
	}

	// Merge user-supplied values over pack defaults.
	merged := mergeValues(pack.Values, values)
	_ = merged // Will be used for template rendering in future implementation.

	// Stub: read each manifest to ensure it exists and is accessible.
	for _, rel := range pack.Manifests {
		absPath := filepath.Join(".", rel)
		if _, err := os.ReadFile(absPath); err != nil {
			return fmt.Errorf("packs: cannot read manifest %s: %w", rel, err)
		}
	}

	// TODO: render templates with merged values and apply to cluster via client-go.
	_ = ctx
	return nil
}

// mergeValues overlays user-provided values on top of defaults.
// User values take precedence.
func mergeValues(defaults, overrides map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range defaults {
		result[k] = v
	}
	for k, v := range overrides {
		result[k] = v
	}
	return result
}

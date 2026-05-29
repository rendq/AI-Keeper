// Package packs provides industry pack format definitions and loading utilities.
// An industry pack bundles YAML manifests, default values, and integrity checksums
// into a portable unit that can be installed into a tenant namespace.
// Covers requirement C5.
package packs

// PackMeta holds descriptive metadata for an industry pack.
type PackMeta struct {
	// Name is the unique pack identifier (e.g. "industry/finance").
	Name string `yaml:"name" json:"name"`

	// Version follows semver (e.g. "1.2.0").
	Version string `yaml:"version" json:"version"`

	// Description is a human-readable summary.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Author is the pack publisher or maintainer.
	Author string `yaml:"author,omitempty" json:"author,omitempty"`

	// Dependencies lists other packs this pack depends on (name@version).
	Dependencies []string `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`

	// Tags are searchable labels for pack discovery.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// PackManifest represents the full contents of a pack directory:
//
//	pack.yaml          — metadata
//	manifests/*.yaml   — Kubernetes manifests to apply
//	values.yaml        — default value overrides
//	checksums.txt      — SHA-256 integrity hashes
//	signatures/*.sig   — optional signature files
type PackManifest struct {
	// Meta is the pack-level metadata loaded from pack.yaml.
	Meta PackMeta `yaml:"meta" json:"meta"`

	// Manifests lists relative paths to YAML files under manifests/.
	Manifests []string `yaml:"manifests,omitempty" json:"manifests,omitempty"`

	// Values holds the default configuration values from values.yaml.
	Values map[string]interface{} `yaml:"values,omitempty" json:"values,omitempty"`

	// Checksums maps file paths to their expected SHA-256 hex digests.
	Checksums map[string]string `yaml:"checksums,omitempty" json:"checksums,omitempty"`
}

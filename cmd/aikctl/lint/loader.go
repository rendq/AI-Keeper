// Package lint implements the aikctl lint rules engine.
//
// It loads YAML resources and runs P0 lint rules against them.
// Requirements: A9.2 (subset)
package lint

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Resource is a generic parsed YAML resource with its source file.
type Resource struct {
	// File is the source YAML file path.
	File string

	// APIVersion from the YAML metadata.
	APIVersion string `yaml:"apiVersion"`

	// Kind of the resource.
	Kind string `yaml:"kind"`

	// Metadata contains name, namespace, labels, annotations.
	Metadata ResourceMetadata `yaml:"metadata"`

	// Spec is the raw spec node for rule-specific parsing.
	Spec yaml.Node `yaml:"spec"`

	// RawSpec is the decoded spec as a generic map for easy field access.
	RawSpec map[string]interface{}
}

// ResourceMetadata holds standard K8s metadata fields.
type ResourceMetadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

// ResourceSet holds all loaded resources grouped by kind.
type ResourceSet struct {
	All    []Resource
	Skills []Resource
	Agents []Resource
	Tools  []Resource
	Policies []Resource
}

// LoadResources reads and parses YAML files into a ResourceSet.
func LoadResources(files []string) (*ResourceSet, error) {
	rs := &ResourceSet{}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}

		// Split multi-document YAML.
		docs := splitYAMLDocuments(string(data))
		for _, doc := range docs {
			if strings.TrimSpace(doc) == "" {
				continue
			}

			var r Resource
			if err := yaml.Unmarshal([]byte(doc), &r); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", file, err)
			}

			// Also decode spec as generic map.
			var full struct {
				Spec map[string]interface{} `yaml:"spec"`
			}
			_ = yaml.Unmarshal([]byte(doc), &full)
			r.RawSpec = full.Spec
			r.File = file

			if r.Kind == "" {
				continue
			}

			rs.All = append(rs.All, r)
			switch r.Kind {
			case "Skill":
				rs.Skills = append(rs.Skills, r)
			case "Agent":
				rs.Agents = append(rs.Agents, r)
			case "Tool":
				rs.Tools = append(rs.Tools, r)
			case "Policy":
				rs.Policies = append(rs.Policies, r)
			}
		}
	}

	return rs, nil
}

// splitYAMLDocuments splits on --- document separators.
func splitYAMLDocuments(content string) []string {
	parts := strings.Split(content, "\n---")
	// The first part doesn't have a leading ---, keep it as is.
	// Subsequent parts had "---" stripped from their start.
	return parts
}

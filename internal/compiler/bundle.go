package compiler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"time"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// manifestJSON is the OPA bundle manifest structure.
type manifestJSON struct {
	Revision string   `json:"revision"`
	Roots    []string `json:"roots"`
}

// dataJSONRoot is the top-level structure of data.json in the bundle.
type dataJSONRoot struct {
	AIP dataJSONAIP `json:"aip"`
}

// dataJSONAIP contains the aip namespace data.
type dataJSONAIP struct {
	Context    dataJSONContext     `json:"context"`
	CelResults map[string]bool    `json:"cel_results,omitempty"`
	Metadata   dataJSONMetadata   `json:"metadata"`
}

// dataJSONContext holds subject/resource context data for Rego evaluation.
type dataJSONContext struct {
	Subjects  []subjectData  `json:"subjects,omitempty"`
	Resources []resourceData `json:"resources,omitempty"`
}

// subjectData is the data.json representation of a cached subject.
type subjectData struct {
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// resourceData is the data.json representation of an indexed resource.
type resourceData struct {
	Kind           string            `json:"kind"`
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Classification string            `json:"classification,omitempty"`
}

// dataJSONMetadata holds bundle metadata.
type dataJSONMetadata struct {
	Version   int64  `json:"version"`
	CompiledAt string `json:"compiled_at"`
	PolicyCount int   `json:"policy_count"`
}

// generateDataJSON produces the data.json content for the OPA bundle.
func generateDataJSON(policies []policyv1alpha1.Policy, subjects []SubjectCacheEntry, resources []ResourceIndexEntry) ([]byte, error) {
	data := dataJSONRoot{
		AIP: dataJSONAIP{
			Context: dataJSONContext{
				Subjects:  convertSubjects(subjects),
				Resources: convertResources(resources),
			},
			CelResults: make(map[string]bool),
			Metadata: dataJSONMetadata{
				CompiledAt:  time.Now().UTC().Format(time.RFC3339),
				PolicyCount: len(policies),
			},
		},
	}

	return json.MarshalIndent(data, "", "  ")
}

// convertSubjects converts SubjectCacheEntry slice to data.json format.
func convertSubjects(subjects []SubjectCacheEntry) []subjectData {
	if len(subjects) == 0 {
		return nil
	}
	result := make([]subjectData, len(subjects))
	for i, s := range subjects {
		result[i] = subjectData{
			Kind:      s.Kind,
			Name:      s.Name,
			Namespace: s.Namespace,
			Labels:    s.Labels,
		}
	}
	return result
}

// convertResources converts ResourceIndexEntry slice to data.json format.
func convertResources(resources []ResourceIndexEntry) []resourceData {
	if len(resources) == 0 {
		return nil
	}
	result := make([]resourceData, len(resources))
	for i, r := range resources {
		result[i] = resourceData{
			Kind:           r.Kind,
			Name:           r.Name,
			Namespace:      r.Namespace,
			Labels:         r.Labels,
			Classification: r.Classification,
		}
	}
	return result
}

// generateManifest creates the OPA bundle manifest content.
func generateManifest(version int64) []byte {
	m := manifestJSON{
		Revision: fmt.Sprintf("%d", version),
		Roots:    []string{"aip"},
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return data
}

// packBundle creates a tar.gz archive containing .rego files, data.json, and .manifest.
func packBundle(regoFiles []regoFile, dataJSON []byte, manifest []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Write .rego files
	for _, rf := range regoFiles {
		if err := writeToTar(tw, rf.Name, rf.Content); err != nil {
			return nil, fmt.Errorf("writing rego file %s: %w", rf.Name, err)
		}
	}

	// Write data.json
	if err := writeToTar(tw, "data.json", dataJSON); err != nil {
		return nil, fmt.Errorf("writing data.json: %w", err)
	}

	// Write .manifest (OPA bundle standard)
	if err := writeToTar(tw, ".manifest", manifest); err != nil {
		return nil, fmt.Errorf("writing .manifest: %w", err)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// writeToTar adds a file entry to the tar archive.
func writeToTar(tw *tar.Writer, name string, content []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // deterministic
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(content)
	return err
}

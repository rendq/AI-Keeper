package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultRegistry is the default container registry for AIP images.
const DefaultRegistry = "ghcr.io/aip-io"

// DefaultTag is the default image tag.
const DefaultTag = "latest"

// ComponentImages lists all AIP component image names.
var ComponentImages = []string{
	"aip-controller",
	"aik-pdp",
	"aik-gateway",
	"aik-audit",
	"aik-router",
	"aik-marketplace",
	"aip-console",
}

// ImageManifest describes the set of images to include in the bundle.
type ImageManifest struct {
	Registry string
	Tag      string
	Images   []string
}

// BundleConfig defines the configuration for creating an airgap bundle.
type BundleConfig struct {
	OutputPath         string
	Images             []string
	HelmChartPath      string
	CRDPath            string
	BundleTemplatePath string
}

// DockerClient abstracts docker operations for testability.
type DockerClient interface {
	// SaveImages exports the given images to a tar stream written to w.
	SaveImages(ctx context.Context, images []string, w io.Writer) error
}

// RealDockerClient implements DockerClient using the docker CLI.
type RealDockerClient struct{}

// SaveImages shells out to `docker save` to export images.
func (r *RealDockerClient) SaveImages(ctx context.Context, images []string, w io.Writer) error {
	// In production, this would exec `docker save <images...>` and pipe to w.
	// Placeholder: actual implementation depends on docker SDK or CLI exec.
	return fmt.Errorf("docker save not implemented in stub; use docker SDK")
}

// Packager orchestrates the airgap bundle creation.
type Packager struct {
	docker DockerClient
}

// NewPackager creates a Packager with the given DockerClient.
func NewPackager(docker DockerClient) *Packager {
	return &Packager{docker: docker}
}

// DefaultManifest returns the default set of AIP component images.
func (p *Packager) DefaultManifest() ImageManifest {
	return ImageManifest{
		Registry: DefaultRegistry,
		Tag:      DefaultTag,
		Images:   ComponentImages,
	}
}

// CollectImages resolves image references from a manifest into fully qualified image strings.
func (p *Packager) CollectImages(manifest ImageManifest) []string {
	images := make([]string, 0, len(manifest.Images))
	for _, img := range manifest.Images {
		ref := fmt.Sprintf("%s/%s:%s", manifest.Registry, img, manifest.Tag)
		images = append(images, ref)
	}
	return images
}

// Validate checks the BundleConfig for required fields.
func (c *BundleConfig) Validate() error {
	if c.OutputPath == "" {
		return fmt.Errorf("output path is required")
	}
	if len(c.Images) == 0 {
		return fmt.Errorf("at least one image is required")
	}
	return nil
}

// CreateBundle builds the airgap tar.gz bundle containing images, charts, CRDs, and templates.
func (p *Packager) CreateBundle(ctx context.Context, config BundleConfig) (string, error) {
	if err := config.Validate(); err != nil {
		return "", fmt.Errorf("invalid config: %w", err)
	}

	outFile, err := os.Create(config.OutputPath)
	if err != nil {
		return "", fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Save docker images into bundle
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.docker.SaveImages(ctx, config.Images, pw)
		pw.Close()
	}()

	if err := addStreamToTar(tarWriter, "images.tar", pr); err != nil {
		return "", fmt.Errorf("add images to bundle: %w", err)
	}
	if err := <-errCh; err != nil {
		return "", fmt.Errorf("docker save: %w", err)
	}

	// Add Helm chart directory
	if config.HelmChartPath != "" {
		if err := addDirToTar(tarWriter, config.HelmChartPath, "helm/"); err != nil {
			return "", fmt.Errorf("add helm chart: %w", err)
		}
	}

	// Add CRD YAML directory
	if config.CRDPath != "" {
		if err := addDirToTar(tarWriter, config.CRDPath, "crds/"); err != nil {
			return "", fmt.Errorf("add crds: %w", err)
		}
	}

	// Add PDP bundle template
	if config.BundleTemplatePath != "" {
		if err := addDirToTar(tarWriter, config.BundleTemplatePath, "pdp-bundle/"); err != nil {
			return "", fmt.Errorf("add pdp bundle template: %w", err)
		}
	}

	return config.OutputPath, nil
}

// addStreamToTar reads all data from r and adds it as a single file entry in the tar.
func addStreamToTar(tw *tar.Writer, name string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

// addDirToTar walks a directory and adds all files to the tar under the given prefix.
func addDirToTar(tw *tar.Writer, srcDir, prefix string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		header := &tar.Header{
			Name: prefix + relPath,
			Size: info.Size(),
			Mode: int64(info.Mode()),
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}

// Package main provides the airgap install orchestration for aikctl.
// It supports offline (air-gapped) Kubernetes cluster installations by:
// loading images → deploying a local registry → helm install → verifying health.
//
// Requirements: C6.5
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CommandExecutor abstracts shell command execution for testability.
type CommandExecutor interface {
	// Execute runs a command with arguments and returns combined output and error.
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
}

// AirgapInstallConfig holds configuration for an air-gapped installation.
type AirgapInstallConfig struct {
	// BundlePath is the path to the airgap bundle tarball produced by aik-airgap-pack.
	BundlePath string
	// Namespace is the target Kubernetes namespace for deployment.
	Namespace string
	// RegistryEndpoint is the in-cluster registry address (e.g., "registry.aik-system.svc:5000").
	RegistryEndpoint string
	// HelmReleaseName is the Helm release name for the AIP installation.
	HelmReleaseName string
}

// AirgapInstaller orchestrates an air-gapped AIP installation on a Kubernetes cluster.
type AirgapInstaller struct {
	Executor CommandExecutor
}

// NewAirgapInstaller creates a new AirgapInstaller with the given executor.
func NewAirgapInstaller(executor CommandExecutor) *AirgapInstaller {
	return &AirgapInstaller{Executor: executor}
}

// LoadImages extracts and loads container images from the bundle into the local container runtime.
func (a *AirgapInstaller) LoadImages(ctx context.Context, bundlePath string) error {
	if bundlePath == "" {
		return fmt.Errorf("airgap: bundle path is empty")
	}

	// Verify bundle exists
	if _, err := os.Stat(bundlePath); os.IsNotExist(err) {
		return fmt.Errorf("airgap: bundle not found at %s", bundlePath)
	}

	// Extract images tarball from bundle
	extractDir := filepath.Join(filepath.Dir(bundlePath), "airgap-extracted")
	_, err := a.Executor.Execute(ctx, "tar", "xzf", bundlePath, "-C", filepath.Dir(bundlePath), "--strip-components=1", "-o")
	if err != nil {
		return fmt.Errorf("airgap: failed to extract bundle: %w", err)
	}

	// Load images into container runtime (supports docker and ctr/containerd)
	imagesTar := filepath.Join(extractDir, "images.tar")
	_, err = a.Executor.Execute(ctx, "ctr", "-n", "k8s.io", "images", "import", imagesTar)
	if err != nil {
		// Fallback to docker load
		_, err = a.Executor.Execute(ctx, "docker", "load", "-i", imagesTar)
		if err != nil {
			return fmt.Errorf("airgap: failed to load images: %w", err)
		}
	}

	return nil
}

// DeployRegistry deploys a local container registry within the cluster namespace
// so that images are available for pod pulls without external network access.
func (a *AirgapInstaller) DeployRegistry(ctx context.Context, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("airgap: namespace is empty")
	}

	// Create namespace if not exists
	_, _ = a.Executor.Execute(ctx, "kubectl", "create", "namespace", namespace)

	// Deploy registry using kubectl apply with inline manifest
	registryManifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: aip-registry
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aip-registry
  template:
    metadata:
      labels:
        app: aip-registry
    spec:
      containers:
      - name: registry
        image: registry:2
        ports:
        - containerPort: 5000
---
apiVersion: v1
kind: Service
metadata:
  name: aip-registry
  namespace: %s
spec:
  selector:
    app: aip-registry
  ports:
  - port: 5000
    targetPort: 5000`, namespace, namespace)

	// Write manifest to a temp file and apply
	tmpFile := filepath.Join(os.TempDir(), "aip-registry-manifest.yaml")
	if err := os.WriteFile(tmpFile, []byte(registryManifest), 0644); err != nil {
		return fmt.Errorf("airgap: failed to write registry manifest: %w", err)
	}
	defer os.Remove(tmpFile)

	_, err := a.Executor.Execute(ctx, "kubectl", "apply", "-f", tmpFile)
	if err != nil {
		return fmt.Errorf("airgap: failed to deploy registry: %w", err)
	}

	// Wait for registry to be ready
	_, err = a.Executor.Execute(ctx, "kubectl", "rollout", "status",
		"deployment/aip-registry", "-n", namespace, "--timeout=120s")
	if err != nil {
		return fmt.Errorf("airgap: registry deployment not ready: %w", err)
	}

	return nil
}

// HelmInstall runs helm install for the AIP chart in the target namespace.
func (a *AirgapInstaller) HelmInstall(ctx context.Context, chartPath, releaseName, namespace string) error {
	if chartPath == "" {
		return fmt.Errorf("airgap: chart path is empty")
	}
	if releaseName == "" {
		return fmt.Errorf("airgap: release name is empty")
	}
	if namespace == "" {
		return fmt.Errorf("airgap: namespace is empty")
	}

	_, err := a.Executor.Execute(ctx, "helm", "install", releaseName, chartPath,
		"--namespace", namespace,
		"--create-namespace",
		"--wait",
		"--timeout", "600s",
	)
	if err != nil {
		return fmt.Errorf("airgap: helm install failed: %w", err)
	}

	return nil
}

// VerifyHealth checks that all pods in the namespace are in Ready state.
func (a *AirgapInstaller) VerifyHealth(ctx context.Context, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("airgap: namespace is empty")
	}

	output, err := a.Executor.Execute(ctx, "kubectl", "get", "pods",
		"-n", namespace,
		"-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
	if err != nil {
		return fmt.Errorf("airgap: failed to get pod status: %w", err)
	}

	statuses := strings.Fields(string(output))
	if len(statuses) == 0 {
		return fmt.Errorf("airgap: no pods found in namespace %s", namespace)
	}

	for _, status := range statuses {
		if status != "True" {
			return fmt.Errorf("airgap: not all pods are ready in namespace %s", namespace)
		}
	}

	return nil
}

// Install orchestrates the full air-gapped installation flow:
// 1. Load images from bundle
// 2. Deploy local registry
// 3. Helm install
// 4. Verify health
func (a *AirgapInstaller) Install(ctx context.Context, config AirgapInstallConfig) error {
	if config.BundlePath == "" {
		return fmt.Errorf("airgap: bundle path is required")
	}
	if config.Namespace == "" {
		config.Namespace = "aik-system"
	}
	if config.HelmReleaseName == "" {
		config.HelmReleaseName = "aip"
	}
	if config.RegistryEndpoint == "" {
		config.RegistryEndpoint = fmt.Sprintf("aip-registry.%s.svc:5000", config.Namespace)
	}

	// Step 1: Load images
	if err := a.LoadImages(ctx, config.BundlePath); err != nil {
		return fmt.Errorf("step load-images failed: %w", err)
	}

	// Step 2: Deploy registry
	if err := a.DeployRegistry(ctx, config.Namespace); err != nil {
		return fmt.Errorf("step deploy-registry failed: %w", err)
	}

	// Step 3: Helm install (chart is expected inside the extracted bundle)
	chartPath := filepath.Join(filepath.Dir(config.BundlePath), "airgap-extracted", "chart")
	if err := a.HelmInstall(ctx, chartPath, config.HelmReleaseName, config.Namespace); err != nil {
		return fmt.Errorf("step helm-install failed: %w", err)
	}

	// Step 4: Verify health
	if err := a.VerifyHealth(ctx, config.Namespace); err != nil {
		return fmt.Errorf("step verify-health failed: %w", err)
	}

	return nil
}

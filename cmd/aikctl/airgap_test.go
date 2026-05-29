package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockExecutor implements CommandExecutor for testing.
type mockExecutor struct {
	// calls records all command invocations as "name arg1 arg2 ..."
	calls []string
	// responses maps command prefixes to their mock outputs
	responses map[string]mockResponse
	// defaultErr if set, all commands fail with this error
	defaultErr error
}

type mockResponse struct {
	output []byte
	err    error
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		responses: make(map[string]mockResponse),
	}
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	m.calls = append(m.calls, call)

	if m.defaultErr != nil {
		return nil, m.defaultErr
	}

	// Check for matching response (longest prefix match)
	for prefix, resp := range m.responses {
		if strings.HasPrefix(call, prefix) {
			return resp.output, resp.err
		}
	}

	return []byte(""), nil
}

func (m *mockExecutor) setResponse(prefix string, output []byte, err error) {
	m.responses[prefix] = mockResponse{output: output, err: err}
}

// createTestBundle creates a temporary file to act as a bundle for testing.
func createTestBundle(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "aip-bundle.tar.gz")
	if err := os.WriteFile(bundlePath, []byte("fake-bundle-content"), 0644); err != nil {
		t.Fatal(err)
	}
	return bundlePath
}

func TestAirgapInstall_FullFlow(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()

	// Mock kubectl get pods to return all Ready
	executor.setResponse("kubectl get pods", []byte("True True True"), nil)

	bundlePath := createTestBundle(t)

	installer := NewAirgapInstaller(executor)
	config := AirgapInstallConfig{
		BundlePath:      bundlePath,
		Namespace:       "aik-system",
		HelmReleaseName: "aip",
	}

	err := installer.Install(ctx, config)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}

	// Verify all 4 steps were executed
	if len(executor.calls) == 0 {
		t.Fatal("expected command calls, got none")
	}

	// Should have tar extract, ctr/docker load, kubectl create ns, kubectl apply,
	// kubectl rollout, helm install, kubectl get pods
	hasExtract := false
	hasHelm := false
	hasVerify := false
	for _, call := range executor.calls {
		if strings.HasPrefix(call, "tar ") {
			hasExtract = true
		}
		if strings.HasPrefix(call, "helm install") {
			hasHelm = true
		}
		if strings.Contains(call, "get pods") {
			hasVerify = true
		}
	}

	if !hasExtract {
		t.Error("expected tar extract call")
	}
	if !hasHelm {
		t.Error("expected helm install call")
	}
	if !hasVerify {
		t.Error("expected kubectl get pods call")
	}
}

func TestAirgapInstall_LoadImages(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	installer := NewAirgapInstaller(executor)

	bundlePath := createTestBundle(t)

	err := installer.LoadImages(ctx, bundlePath)
	if err != nil {
		t.Fatalf("LoadImages() unexpected error: %v", err)
	}

	// Should attempt tar extraction and image load
	if len(executor.calls) < 2 {
		t.Fatalf("expected at least 2 calls (tar + load), got %d: %v", len(executor.calls), executor.calls)
	}

	if !strings.HasPrefix(executor.calls[0], "tar ") {
		t.Errorf("first call should be tar, got: %s", executor.calls[0])
	}
}

func TestAirgapInstall_LoadImages_EmptyPath(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	installer := NewAirgapInstaller(executor)

	err := installer.LoadImages(ctx, "")
	if err == nil {
		t.Fatal("LoadImages() expected error for empty path")
	}
	if !strings.Contains(err.Error(), "bundle path is empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAirgapInstall_LoadImages_MissingBundle(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	installer := NewAirgapInstaller(executor)

	err := installer.LoadImages(ctx, "/nonexistent/bundle.tar.gz")
	if err == nil {
		t.Fatal("LoadImages() expected error for missing bundle")
	}
	if !strings.Contains(err.Error(), "bundle not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAirgapInstall_VerifyHealth_AllReady(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	executor.setResponse("kubectl get pods", []byte("True True True"), nil)

	installer := NewAirgapInstaller(executor)
	err := installer.VerifyHealth(ctx, "aik-system")
	if err != nil {
		t.Fatalf("VerifyHealth() unexpected error: %v", err)
	}
}

func TestAirgapInstall_VerifyHealth_NotReady(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	executor.setResponse("kubectl get pods", []byte("True False True"), nil)

	installer := NewAirgapInstaller(executor)
	err := installer.VerifyHealth(ctx, "aik-system")
	if err == nil {
		t.Fatal("VerifyHealth() expected error for not-ready pods")
	}
	if !strings.Contains(err.Error(), "not all pods are ready") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAirgapInstall_VerifyHealth_NoPods(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	executor.setResponse("kubectl get pods", []byte(""), nil)

	installer := NewAirgapInstaller(executor)
	err := installer.VerifyHealth(ctx, "aik-system")
	if err == nil {
		t.Fatal("VerifyHealth() expected error for no pods")
	}
	if !strings.Contains(err.Error(), "no pods found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAirgapInstall_VerifyHealth_EmptyNamespace(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	installer := NewAirgapInstaller(executor)

	err := installer.VerifyHealth(ctx, "")
	if err == nil {
		t.Fatal("VerifyHealth() expected error for empty namespace")
	}
}

func TestAirgapInstall_HelmFailure(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	executor.setResponse("helm install", nil, fmt.Errorf("helm: release already exists"))
	executor.setResponse("kubectl get pods", []byte("True"), nil)

	bundlePath := createTestBundle(t)
	installer := NewAirgapInstaller(executor)

	config := AirgapInstallConfig{
		BundlePath:      bundlePath,
		Namespace:       "aik-system",
		HelmReleaseName: "aip",
	}

	err := installer.Install(ctx, config)
	if err == nil {
		t.Fatal("Install() expected error on helm failure")
	}
	if !strings.Contains(err.Error(), "helm-install failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAirgapInstall_MissingBundleConfig(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	installer := NewAirgapInstaller(executor)

	config := AirgapInstallConfig{
		BundlePath: "",
	}

	err := installer.Install(ctx, config)
	if err == nil {
		t.Fatal("Install() expected error for empty bundle path")
	}
	if !strings.Contains(err.Error(), "bundle path is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAirgapInstall_DefaultConfig(t *testing.T) {
	ctx := context.Background()
	executor := newMockExecutor()
	executor.setResponse("kubectl get pods", []byte("True"), nil)

	bundlePath := createTestBundle(t)
	installer := NewAirgapInstaller(executor)

	config := AirgapInstallConfig{
		BundlePath: bundlePath,
	}

	err := installer.Install(ctx, config)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}

	// Verify defaults were applied — helm call should include "aik-system" and "aip"
	hasDefaultNs := false
	hasDefaultRelease := false
	for _, call := range executor.calls {
		if strings.Contains(call, "aik-system") {
			hasDefaultNs = true
		}
		if strings.Contains(call, "helm install aip") {
			hasDefaultRelease = true
		}
	}
	if !hasDefaultNs {
		t.Error("expected default namespace 'aik-system' in commands")
	}
	if !hasDefaultRelease {
		t.Error("expected default release name 'aip' in helm install")
	}
}

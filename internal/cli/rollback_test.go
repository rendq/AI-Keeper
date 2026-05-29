package cli

import (
	"context"
	"fmt"
	"testing"
)

// mockAgentClient implements AgentClient for testing.
type mockAgentClient struct {
	agent *AgentInfo
	err   error
	// rolledBack tracks if RollbackAgent was called.
	rolledBack      bool
	rolledBackToRev int64
}

func (m *mockAgentClient) GetAgent(_ context.Context, _, _ string) (*AgentInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.agent, nil
}

func (m *mockAgentClient) RollbackAgent(_ context.Context, _, _ string, revision int64) error {
	m.rolledBack = true
	m.rolledBackToRev = revision
	return nil
}

func TestRunRollback_Success(t *testing.T) {
	client := &mockAgentClient{
		agent: &AgentInfo{
			Name:                   "my-agent",
			Namespace:              "default",
			CurrentRevision:        5,
			LastSuccessfulRevision: 4,
		},
	}

	opts := RollbackOptions{
		AgentName: "my-agent",
		Namespace: "default",
	}

	err := RunRollback(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !client.rolledBack {
		t.Fatal("expected RollbackAgent to be called")
	}
	if client.rolledBackToRev != 4 {
		t.Fatalf("expected rollback to revision 4, got %d", client.rolledBackToRev)
	}
}

func TestRunRollback_NoPreviousRevision(t *testing.T) {
	client := &mockAgentClient{
		agent: &AgentInfo{
			Name:                   "my-agent",
			Namespace:              "default",
			CurrentRevision:        1,
			LastSuccessfulRevision: 0,
		},
	}

	opts := RollbackOptions{
		AgentName: "my-agent",
		Namespace: "default",
	}

	err := RunRollback(context.Background(), client, opts)
	if err == nil {
		t.Fatal("expected error when no previous revision exists")
	}
	expected := "no previous successful revision"
	if !containsSubstring(err.Error(), expected) {
		t.Fatalf("expected error to contain %q, got: %v", expected, err)
	}
	if client.rolledBack {
		t.Fatal("RollbackAgent should not be called when no previous revision exists")
	}
}

func TestRunRollback_AlreadyAtLastRevision(t *testing.T) {
	client := &mockAgentClient{
		agent: &AgentInfo{
			Name:                   "my-agent",
			Namespace:              "default",
			CurrentRevision:        3,
			LastSuccessfulRevision: 3,
		},
	}

	opts := RollbackOptions{
		AgentName: "my-agent",
		Namespace: "default",
	}

	err := RunRollback(context.Background(), client, opts)
	if err == nil {
		t.Fatal("expected error when already at last successful revision")
	}
	expected := "already at the last successful revision"
	if !containsSubstring(err.Error(), expected) {
		t.Fatalf("expected error to contain %q, got: %v", expected, err)
	}
}

func TestRunRollback_DryRun(t *testing.T) {
	client := &mockAgentClient{
		agent: &AgentInfo{
			Name:                   "my-agent",
			Namespace:              "default",
			CurrentRevision:        5,
			LastSuccessfulRevision: 4,
		},
	}

	opts := RollbackOptions{
		AgentName: "my-agent",
		Namespace: "default",
		DryRun:    true,
	}

	err := RunRollback(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client.rolledBack {
		t.Fatal("RollbackAgent should not be called in dry-run mode")
	}
}

func TestRunRollback_GetAgentError(t *testing.T) {
	client := &mockAgentClient{
		err: fmt.Errorf("not found"),
	}

	opts := RollbackOptions{
		AgentName: "missing-agent",
		Namespace: "default",
	}

	err := RunRollback(context.Background(), client, opts)
	if err == nil {
		t.Fatal("expected error when agent cannot be fetched")
	}
}

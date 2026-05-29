package main

import (
	"context"
	"time"

	aipv1 "github.com/ai-keeper/ai-keeper/proto/aip/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// PEPTimeout is the maximum time a PEP client waits for a PDP decision.
	// Per requirement B2.8 / A5.13: 1 second timeout, fail-closed on expiry.
	PEPTimeout = 1 * time.Second
)

// PEPClient wraps the PDP gRPC client with fail-closed semantics.
// If the PDP does not respond within PEPTimeout, the client returns deny.
type PEPClient struct {
	client  aipv1.PolicyDecisionServiceClient
	conn    *grpc.ClientConn
	timeout time.Duration
}

// NewPEPClient creates a new PEP client connected to the given PDP address.
// The default timeout is 1s (PEPTimeout).
func NewPEPClient(addr string) (*PEPClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	return &PEPClient{
		client:  aipv1.NewPolicyDecisionServiceClient(conn),
		conn:    conn,
		timeout: PEPTimeout,
	}, nil
}

// NewPEPClientWithTimeout creates a PEP client with a custom timeout.
func NewPEPClientWithTimeout(addr string, timeout time.Duration) (*PEPClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	return &PEPClient{
		client:  aipv1.NewPolicyDecisionServiceClient(conn),
		conn:    conn,
		timeout: timeout,
	}, nil
}

// newPEPClientFromConn creates a PEP client from an existing gRPC connection (for testing).
func newPEPClientFromConn(conn *grpc.ClientConn, timeout time.Duration) *PEPClient {
	return &PEPClient{
		client:  aipv1.NewPolicyDecisionServiceClient(conn),
		conn:    conn,
		timeout: timeout,
	}
}

// Decide calls the PDP with fail-closed semantics.
// If the PDP doesn't respond within the timeout, returns DENY with reason "PolicyTimeout".
func (c *PEPClient) Decide(ctx context.Context, req *aipv1.DecisionRequest) *aipv1.DecisionResponse {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Decide(ctx, req)
	if err != nil {
		// Fail-closed: any error (including timeout) results in deny.
		return &aipv1.DecisionResponse{
			Decision:    aipv1.Decision_DECISION_DENY,
			Reason:      "PolicyTimeout",
			EvaluatedAt: timestamppb.Now(),
		}
	}

	return resp
}

// Close closes the underlying gRPC connection.
func (c *PEPClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

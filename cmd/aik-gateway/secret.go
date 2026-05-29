package gateway

import (
	"context"
	"errors"
	"sync"
)

// SecretProvider retrieves secrets from a secure store (Vault / KMS).
// Secrets are never stored in ConfigMaps or environment variables.
type SecretProvider interface {
	// GetSecret retrieves a secret by path/key.
	// Returns the secret value or an error if not found or access denied.
	GetSecret(ctx context.Context, path string) (string, error)
}

// StubSecretProvider is a P0 stub that stores secrets in-memory.
// In production, this would be replaced by a Vault or KMS implementation.
type StubSecretProvider struct {
	mu      sync.RWMutex
	secrets map[string]string
}

// NewStubSecretProvider creates a stub provider with initial secrets.
func NewStubSecretProvider(secrets map[string]string) *StubSecretProvider {
	if secrets == nil {
		secrets = make(map[string]string)
	}
	return &StubSecretProvider{secrets: secrets}
}

// GetSecret retrieves a secret from the in-memory store.
func (s *StubSecretProvider) GetSecret(_ context.Context, path string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.secrets[path]
	if !ok {
		return "", errors.New("secret not found: " + path)
	}
	return val, nil
}

// SetSecret stores a secret (for testing purposes).
func (s *StubSecretProvider) SetSecret(path, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[path] = value
}

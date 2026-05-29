//go:build gm

package gm

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"testing"
)

func TestSM3EventHash(t *testing.T) {
	event := map[string]interface{}{
		"eventID":   "evt-001",
		"action":    "model.invoke",
		"principal": "user:alice",
		"resource":  "model:gpt-4",
		"timestamp": "2025-01-01T00:00:00Z",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	hasher := &GMEventHasher{}

	// Hash should produce 32-byte output
	hash1 := hasher.Hash(data)
	if len(hash1) != 32 {
		t.Fatalf("expected 32-byte hash, got %d bytes", len(hash1))
	}

	// Hash should be deterministic
	hash2 := hasher.Hash(data)
	if !bytes.Equal(hash1, hash2) {
		t.Fatal("SM3 hash is not deterministic")
	}

	// Different input should produce different hash
	otherData := []byte("different payload")
	hash3 := hasher.Hash(otherData)
	if bytes.Equal(hash1, hash3) {
		t.Fatal("SM3 hash collision on different inputs")
	}
}

func TestSM4AuditEncryptDecrypt(t *testing.T) {
	encryptor := &GMAuditEncryptor{}

	key := make([]byte, SM4KeySize)
	for i := range key {
		key[i] = byte(i)
	}

	event := []byte(`{"eventID":"evt-002","action":"data.access","principal":"svc:pipeline"}`)

	// Encrypt
	ciphertext, err := encryptor.Encrypt(key, event)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Ciphertext should differ from plaintext
	if bytes.Equal(ciphertext, event) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	// Round-trip should recover original
	if !bytes.Equal(plaintext, event) {
		t.Fatalf("round-trip failed: got %q, want %q", plaintext, event)
	}

	// Wrong key should fail
	wrongKey := make([]byte, SM4KeySize)
	for i := range wrongKey {
		wrongKey[i] = byte(i + 1)
	}
	_, err = encryptor.Decrypt(wrongKey, ciphertext)
	if err == nil {
		t.Fatal("expected decrypt to fail with wrong key")
	}

	// Invalid key size should fail
	_, err = encryptor.Encrypt([]byte("short"), event)
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
}

func TestSM2TLSConfig(t *testing.T) {
	cfg := GMTLSConfig()

	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion TLS 1.3 (%d), got %d", tls.VersionTLS13, cfg.MinVersion)
	}

	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected ClientAuth RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
	}

	// Verify constants are defined
	if GMCipherSuite == 0 {
		t.Fatal("GMCipherSuite should be non-zero")
	}

	if GMTLSVersion != tls.VersionTLS13 {
		t.Fatalf("GMTLSVersion should equal TLS 1.3, got %d", GMTLSVersion)
	}
}

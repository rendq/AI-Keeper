//go:build gm

package gm

import (
	"crypto/tls"
)

// Constants for GM TLS configuration.
const (
	// GMCipherSuite represents the SM2-SM4-SM3 cipher suite identifier.
	// Note: This is a placeholder value; real SM2 cipher suites require TLCP protocol support.
	GMCipherSuite uint16 = 0xe013

	// GMTLSVersion represents the minimum TLS version for GM compliance.
	// We use TLS 1.3 as the base, with SM2 cipher negotiation handled at the application layer.
	GMTLSVersion uint16 = tls.VersionTLS13
)

// GMEventHasher implements audit event hashing using SM3.
type GMEventHasher struct{}

// Hash computes the SM3 hash of the given data, returning a 32-byte digest.
func (h *GMEventHasher) Hash(data []byte) []byte {
	return SM3Hash(data)
}

// GMAuditEncryptor provides SM4-GCM encryption/decryption for audit event payloads.
type GMAuditEncryptor struct{}

// Encrypt encrypts an audit event payload using SM4-GCM.
// Key must be exactly 16 bytes. Returns nonce || ciphertext.
func (e *GMAuditEncryptor) Encrypt(key, event []byte) ([]byte, error) {
	return SM4Encrypt(key, event)
}

// Decrypt decrypts an SM4-GCM encrypted audit event payload.
// Key must be exactly 16 bytes. Expects nonce || ciphertext format.
func (e *GMAuditEncryptor) Decrypt(key, ciphertext []byte) ([]byte, error) {
	return SM4Decrypt(key, ciphertext)
}

// GMTLSConfig returns a *tls.Config configured for GM cryptographic compliance.
// Note: Full SM2 certificate-based mTLS requires TLCP (GM/T 0024) protocol support,
// which is not natively available in Go's crypto/tls. This configuration sets
// TLS 1.3 as minimum version and documents the intended SM2 cipher suite.
// For production TLCP support, use a GM-aware TLS library (e.g., gmtls).
func GMTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: GMTLSVersion,
		// SM2 cipher suites are negotiated via TLCP protocol extension.
		// Standard TLS 1.3 cipher suites are used as fallback.
		CipherSuites: nil, // TLS 1.3 manages its own cipher suites
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
}

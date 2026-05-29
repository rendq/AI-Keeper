//go:build gm

// Package gm provides Chinese national cryptographic algorithm (国密) implementations
// for SM2 (asymmetric), SM3 (hash), and SM4 (symmetric) as alternatives to
// RSA/ECDSA, SHA-256, and AES-256-GCM respectively.
package gm

import (
	"crypto/ecdsa"
	"crypto/rand"

	"github.com/emmansun/gmsm/sm2"
)

// SM2PrivateKey wraps the SM2 private key.
type SM2PrivateKey struct {
	key *sm2.PrivateKey
}

// SM2PublicKey wraps the SM2 public key.
type SM2PublicKey struct {
	key *ecdsa.PublicKey
}

// Public returns the public key corresponding to this private key.
func (priv *SM2PrivateKey) Public() *SM2PublicKey {
	return &SM2PublicKey{key: &priv.key.PublicKey}
}

// SM2GenerateKey generates a new SM2 key pair.
func SM2GenerateKey() (*SM2PrivateKey, error) {
	priv, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &SM2PrivateKey{key: priv}, nil
}

// SM2Sign signs msg using the SM2 private key with SM2-specific signing (uid default).
func SM2Sign(priv *SM2PrivateKey, msg []byte) ([]byte, error) {
	return priv.key.SignWithSM2(rand.Reader, nil, msg)
}

// SM2Verify verifies an SM2 signature against msg using the public key.
func SM2Verify(pub *SM2PublicKey, msg, sig []byte) bool {
	return sm2.VerifyASN1WithSM2(pub.key, nil, msg, sig)
}

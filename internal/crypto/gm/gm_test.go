//go:build gm

package gm

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestSM2SignVerify(t *testing.T) {
	priv, err := SM2GenerateKey()
	if err != nil {
		t.Fatalf("SM2GenerateKey failed: %v", err)
	}

	msg := []byte("hello SM2 国密签名测试")
	sig, err := SM2Sign(priv, msg)
	if err != nil {
		t.Fatalf("SM2Sign failed: %v", err)
	}

	pub := priv.Public()
	if !SM2Verify(pub, msg, sig) {
		t.Fatal("SM2Verify failed: valid signature rejected")
	}

	// Tamper with message — should fail verification
	tampered := append([]byte{}, msg...)
	tampered[0] ^= 0xff
	if SM2Verify(pub, tampered, sig) {
		t.Fatal("SM2Verify should reject tampered message")
	}
}

func TestSM3HashDeterminism(t *testing.T) {
	data := []byte("SM3 determinism test 国密哈希")
	h1 := SM3Hash(data)
	h2 := SM3Hash(data)

	if !bytes.Equal(h1, h2) {
		t.Fatal("SM3Hash is not deterministic")
	}
}

func TestSM3HashLength(t *testing.T) {
	data := []byte("test")
	h := SM3Hash(data)
	if len(h) != 32 {
		t.Fatalf("SM3Hash output length = %d, want 32", len(h))
	}
}

func TestSM3Streaming(t *testing.T) {
	data := []byte("streaming hash test")
	// One-shot
	oneShot := SM3Hash(data)

	// Streaming
	h := SM3New()
	h.Write(data[:8])
	h.Write(data[8:])
	streamed := h.Sum(nil)

	if !bytes.Equal(oneShot, streamed) {
		t.Fatal("SM3 streaming hash differs from one-shot")
	}
}

func TestSM4EncryptDecrypt(t *testing.T) {
	key := make([]byte, SM4KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("SM4-GCM 对称加密测试")
	ciphertext, err := SM4Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("SM4Encrypt failed: %v", err)
	}

	decrypted, err := SM4Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("SM4Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatal("SM4 decrypt does not match original plaintext")
	}
}

func TestSM4WrongKey(t *testing.T) {
	key := make([]byte, SM4KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("wrong key test")
	ciphertext, err := SM4Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("SM4Encrypt failed: %v", err)
	}

	// Use a different key to decrypt
	wrongKey := make([]byte, SM4KeySize)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatal(err)
	}

	_, err = SM4Decrypt(wrongKey, ciphertext)
	if err == nil {
		t.Fatal("SM4Decrypt should fail with wrong key")
	}
}

func TestSM4InvalidKeySize(t *testing.T) {
	badKey := make([]byte, 15) // not 16 bytes
	_, err := SM4Encrypt(badKey, []byte("data"))
	if err != ErrInvalidKeySize {
		t.Fatalf("expected ErrInvalidKeySize, got %v", err)
	}

	_, err = SM4Decrypt(badKey, []byte("data"))
	if err != ErrInvalidKeySize {
		t.Fatalf("expected ErrInvalidKeySize, got %v", err)
	}
}

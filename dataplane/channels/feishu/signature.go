package feishu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
)

// verifySignature verifies the Feishu webhook signature.
// Feishu uses HMAC-SHA256 with the formula:
//
//	signature = sha256(timestamp + nonce + appSecret + body)
//
// The signature, timestamp, and nonce can come from headers or from the event body token.
func (a *Adapter) verifySignature(r *http.Request, body []byte, env *eventEnvelope) error {
	// Try header-based verification first (custom headers for newer API versions).
	timestamp := r.Header.Get("X-Lark-Request-Timestamp")
	nonce := r.Header.Get("X-Lark-Request-Nonce")
	signature := r.Header.Get("X-Lark-Signature")

	if signature != "" && timestamp != "" && nonce != "" {
		expected := computeSignature(timestamp, nonce, a.appSecret, body)
		if !hmac.Equal([]byte(signature), []byte(expected)) {
			return errors.New("signature mismatch")
		}
		return nil
	}

	// Fall back to token-based verification (v2 header token matches verify token).
	if env.Header != nil && env.Header.Token != "" {
		if a.verifyToken != "" && env.Header.Token != a.verifyToken {
			return errors.New("invalid event token")
		}
		return nil
	}

	// V1 token verification.
	if env.Token != "" {
		if a.verifyToken != "" && env.Token != a.verifyToken {
			return errors.New("invalid event token")
		}
		return nil
	}

	return errors.New("no signature or token found")
}

// computeSignature computes the Feishu webhook signature.
// Formula: sha256(timestamp + nonce + appSecret + body)
func computeSignature(timestamp, nonce, appSecret string, body []byte) string {
	content := fmt.Sprintf("%s%s%s%s", timestamp, nonce, appSecret, string(body))
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature is the exported version for external use (e.g., gateway integration).
// It verifies a Feishu webhook request given the app secret.
func VerifySignature(timestamp, nonce, appSecret string, body []byte, expectedSignature string) error {
	computed := computeSignature(timestamp, nonce, appSecret, body)
	if !hmac.Equal([]byte(computed), []byte(expectedSignature)) {
		return errors.New("signature verification failed")
	}
	return nil
}

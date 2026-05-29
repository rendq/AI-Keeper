// Package main implements a minimal OIDC Identity Provider for testing.
// It exposes /.well-known/openid-configuration, /token, and /keys (JWKS) endpoints.
package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"
)

var (
	signingKey *rsa.PrivateKey
	keyID      = "mock-key-1"
)

func init() {
	var err error
	signingKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("failed to generate RSA key: %v", err)
	}
}

// OpenIDConfiguration represents the OIDC discovery document.
type OpenIDConfiguration struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	JWKSURI                string   `json:"jwks_uri"`
	SubjectTypesSupported  []string `json:"subject_types_supported"`
	IDTokenSigningAlg      []string `json:"id_token_signing_alg_values_supported"`
	ResponseTypesSupported []string `json:"response_types_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a single JSON Web Key.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// TokenResponse represents an OAuth2 token response.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token"`
}

func issuerURL() string {
	url := os.Getenv("ISSUER_URL")
	if url == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8081"
		}
		url = fmt.Sprintf("http://localhost:%s", port)
	}
	return url
}

func handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	issuer := issuerURL()
	config := OpenIDConfiguration{
		Issuer:                 issuer,
		AuthorizationEndpoint:  issuer + "/authorize",
		TokenEndpoint:          issuer + "/token",
		JWKSURI:                issuer + "/keys",
		SubjectTypesSupported:  []string{"public"},
		IDTokenSigningAlg:      []string{"RS256"},
		ResponseTypesSupported: []string{"code", "id_token"},
		ScopesSupported:        []string{"openid", "email", "profile"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func handleJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := signingKey.PublicKey
	jwks := JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Use: "sig",
				Kid: keyID,
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jwks)
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate a real JWT signed with RSA-SHA256
	header := base64.RawURLEncoding.EncodeToString([]byte(
		fmt.Sprintf(`{"alg":"RS256","kid":"%s","typ":"JWT"}`, keyID),
	))

	now := time.Now()
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"iss":"%s","sub":"mock-user","aud":"mock-client","exp":%d,"iat":%d,"email":"mock@example.com","name":"Mock User"}`,
		issuerURL(), now.Add(time.Hour).Unix(), now.Unix(),
	)))

	signInput := header + "." + payload
	hash := sha256.Sum256([]byte(signInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, signingKey, crypto.SHA256, hash[:])
	if err != nil {
		http.Error(w, fmt.Sprintf("signing error: %v", err), http.StatusInternalServerError)
		return
	}
	signature := base64.RawURLEncoding.EncodeToString(sig)

	idToken := signInput + "." + signature

	resp := TokenResponse{
		AccessToken: fmt.Sprintf("mock-access-token-%d", now.Unix()),
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		IDToken:     idToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", handleDiscovery)
	mux.HandleFunc("/keys", handleJWKS)
	mux.HandleFunc("/token", handleToken)
	mux.HandleFunc("/healthz", handleHealth)

	log.Printf("mock-idp listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

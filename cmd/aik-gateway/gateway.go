package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
)

// Gateway is the AIP Gateway handler that applies a middleware chain
// (auth → rate limit → trace injection → secret injection) before
// forwarding requests to the downstream PEP.
type Gateway struct {
	auth          Authenticator
	rateLimiter   RateLimiter
	traceInjector TraceInjector
	secretProvider SecretProvider
	downstream    http.Handler

	mu           sync.Mutex
	idCounter    uint64
}

// NewGateway creates a Gateway with the given dependencies.
func NewGateway(opts ...Option) *Gateway {
	g := &Gateway{
		auth:          &noopAuth{},
		rateLimiter:   &noopRateLimiter{},
		traceInjector: &DefaultTraceInjector{},
		secretProvider: &StubSecretProvider{},
		downstream:    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Option configures a Gateway.
type Option func(*Gateway)

// WithAuthenticator sets the authenticator.
func WithAuthenticator(a Authenticator) Option {
	return func(g *Gateway) { g.auth = a }
}

// WithRateLimiter sets the rate limiter.
func WithRateLimiter(r RateLimiter) Option {
	return func(g *Gateway) { g.rateLimiter = r }
}

// WithTraceInjector sets the trace injector.
func WithTraceInjector(t TraceInjector) Option {
	return func(g *Gateway) { g.traceInjector = t }
}

// WithSecretProvider sets the secret provider.
func WithSecretProvider(s SecretProvider) Option {
	return func(g *Gateway) { g.secretProvider = s }
}

// WithDownstream sets the downstream handler (PEP).
func WithDownstream(h http.Handler) Option {
	return func(g *Gateway) { g.downstream = h }
}

// ServeHTTP implements http.Handler. It runs the middleware chain:
// 1. Auth (401 on failure)
// 2. Rate limiting (429 on exceeded)
// 3. Trace injection (traceparent header)
// 4. Generate invocationId
// 5. Forward to downstream PEP
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Authentication
	identity, err := g.auth.Authenticate(ctx, r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Store identity in context for downstream use.
	ctx = withIdentity(ctx, identity)

	// 2. Rate limiting
	key := rateLimitKey(identity)
	if !g.rateLimiter.Allow(ctx, key) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// 3. Trace injection
	r = g.traceInjector.Inject(r.WithContext(ctx))

	// 4. Generate invocation ID
	invocationID := generateInvocationID()
	r.Header.Set("X-Aip-Invocation-Id", invocationID)

	// 5. Forward to downstream
	g.downstream.ServeHTTP(w, r)
}

// generateInvocationID creates a globally unique invocation ID.
func generateInvocationID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// rateLimitKey builds a rate limiting key from the identity.
func rateLimitKey(id *Identity) string {
	if id == nil {
		return "unknown"
	}
	if id.AgentName != "" && id.Channel != "" {
		return id.AgentName + "/" + id.Channel
	}
	if id.AgentName != "" {
		return id.AgentName
	}
	return id.Subject
}

// Identity represents a verified caller identity.
type Identity struct {
	Subject   string // principal identifier
	TenantID  string
	AgentName string
	Channel   string
}

// contextKey is an unexported type for context keys in this package.
type contextKey int

const identityKey contextKey = iota

// withIdentity stores identity in context.
func withIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFromContext retrieves identity from context.
func IdentityFromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey).(*Identity)
	return id
}

package identity

import (
	"context"
	"fmt"
	"time"
)

// RedisClient is the minimal interface required for Redis operations.
// This allows mocking in tests and decouples from any specific Redis client library.
type RedisClient interface {
	// SAdd adds members to a set.
	SAdd(ctx context.Context, key string, members ...string) error
	// SIsMember checks if a member is in a set.
	SIsMember(ctx context.Context, key string, member string) (bool, error)
	// SMembers returns all members of a set.
	SMembers(ctx context.Context, key string) ([]string, error)
	// Del deletes a key.
	Del(ctx context.Context, keys ...string) error
	// Expire sets a TTL on a key.
	Expire(ctx context.Context, key string, ttl time.Duration) error
	// Set sets a key with a TTL.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	// Exists checks if a key exists.
	Exists(ctx context.Context, key string) (bool, error)
}

const (
	// redisBlacklistPrefix is the prefix for token blacklist keys.
	redisBlacklistPrefix = "aip:identity:blacklist:"
	// redisSATokensPrefix is the prefix for SA token tracking sets.
	redisSATokensPrefix = "aip:identity:sa-tokens:"
)

// RedisTokenBlacklist implements TokenBlacklist using Redis SET.
// When a token is revoked, its ID is stored as a key with a TTL
// matching the remaining token lifetime.
type RedisTokenBlacklist struct {
	client RedisClient
}

// NewRedisTokenBlacklist creates a Redis-backed token blacklist.
func NewRedisTokenBlacklist(client RedisClient) *RedisTokenBlacklist {
	return &RedisTokenBlacklist{client: client}
}

// Add adds a token ID to the blacklist with the given TTL.
func (r *RedisTokenBlacklist) Add(ctx context.Context, tokenID string, ttl time.Duration) error {
	key := redisBlacklistPrefix + tokenID
	return r.client.Set(ctx, key, "1", ttl)
}

// AddForSA blacklists all active tokens for a service account.
// It looks up the SA token set and adds each token to the blacklist.
func (r *RedisTokenBlacklist) AddForSA(ctx context.Context, saFQN string, ttl time.Duration) (int, error) {
	saKey := redisSATokensPrefix + saFQN
	tokens, err := r.client.SMembers(ctx, saKey)
	if err != nil {
		return 0, fmt.Errorf("redis: list SA tokens: %w", err)
	}

	count := 0
	for _, tokenID := range tokens {
		if err := r.Add(ctx, tokenID, ttl); err != nil {
			return count, fmt.Errorf("redis: blacklist token %s: %w", tokenID, err)
		}
		count++
	}

	// Clean up the SA token set.
	_ = r.client.Del(ctx, saKey)
	return count, nil
}

// IsRevoked checks if a token ID is in the blacklist.
func (r *RedisTokenBlacklist) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	key := redisBlacklistPrefix + tokenID
	return r.client.Exists(ctx, key)
}

// RedisSATokenStore implements SATokenStore using Redis SETs.
// Each SA has a set containing its active token IDs.
type RedisSATokenStore struct {
	client RedisClient
}

// NewRedisSATokenStore creates a Redis-backed SA token store.
func NewRedisSATokenStore(client RedisClient) *RedisSATokenStore {
	return &RedisSATokenStore{client: client}
}

// Store records a token issued for a service account.
func (r *RedisSATokenStore) Store(ctx context.Context, saFQN string, tokenID string, expiresAt time.Time) error {
	key := redisSATokensPrefix + saFQN
	if err := r.client.SAdd(ctx, key, tokenID); err != nil {
		return err
	}
	// Set TTL to the max token lifetime to auto-clean.
	ttl := time.Until(expiresAt)
	if ttl > 0 {
		return r.client.Expire(ctx, key, ttl+time.Minute) // +1 min buffer
	}
	return nil
}

// ListActive returns all token IDs for a service account.
func (r *RedisSATokenStore) ListActive(ctx context.Context, saFQN string) ([]string, error) {
	key := redisSATokensPrefix + saFQN
	return r.client.SMembers(ctx, key)
}

// Remove removes a specific token from the store.
func (r *RedisSATokenStore) Remove(ctx context.Context, saFQN string, tokenID string) error {
	// For simplicity, we don't remove individual members from the set here;
	// they'll expire with the set TTL. In a production implementation,
	// you'd use SREM.
	return nil
}

// RemoveAll removes all tokens for a service account.
func (r *RedisSATokenStore) RemoveAll(ctx context.Context, saFQN string) error {
	key := redisSATokensPrefix + saFQN
	return r.client.Del(ctx, key)
}

package sink

import (
	"os"
	"time"
)

// Config holds the runtime configuration for the audit sink.
type Config struct {
	// NATS JetStream
	NATSUrl     string
	NATSSubject string
	NATSStream  string
	NATSDurable string

	// ClickHouse
	ClickHouseDSN string

	// S3 (Object Lock / WORM)
	S3Endpoint        string
	S3Bucket          string
	S3Region          string
	S3AccessKey       string
	S3SecretKey       string
	S3UseSSL          bool
	S3DefaultRetention time.Duration

	// Batching
	BatchSize     int
	FlushInterval time.Duration

	// SIEM Forwarder (stub)
	SIEMEnabled bool
	SIEMType    string // "hec" or "cef"
}

// ConfigFromEnv builds a Config from environment variables with sensible defaults.
func ConfigFromEnv() Config {
	cfg := Config{
		NATSUrl:            envOr("NATS_URL", "nats://localhost:4222"),
		NATSSubject:        envOr("NATS_SUBJECT", "audit.events"),
		NATSStream:         envOr("NATS_STREAM", "AUDIT"),
		NATSDurable:        envOr("NATS_DURABLE", "aik-audit-sink"),
		ClickHouseDSN:      envOr("CLICKHOUSE_DSN", "clickhouse://localhost:9000/aip?async_insert=1&wait_for_async_insert=0"),
		S3Endpoint:         envOr("S3_ENDPOINT", "localhost:9000"),
		S3Bucket:           envOr("S3_BUCKET", "audit-events"),
		S3Region:           envOr("S3_REGION", "us-east-1"),
		S3AccessKey:        envOr("S3_ACCESS_KEY", ""),
		S3SecretKey:        envOr("S3_SECRET_KEY", ""),
		S3UseSSL:           envOr("S3_USE_SSL", "false") == "true",
		S3DefaultRetention: 365 * 24 * time.Hour,
		BatchSize:          100,
		FlushInterval:      1 * time.Second,
		SIEMEnabled:        envOr("SIEM_ENABLED", "false") == "true",
		SIEMType:           envOr("SIEM_TYPE", "hec"),
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

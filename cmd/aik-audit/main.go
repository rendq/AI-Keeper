// Package main provides the aik-audit binary — an audit event sink that
// consumes from NATS JetStream topic "audit.events" and dual-writes to
// ClickHouse (async batch insert) and S3 (Object Lock Compliance Mode / WORM).
//
// Requirements: B1.6, B12.1, B12.6, F4, F5
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ai-keeper/ai-keeper/cmd/aik-audit/sink"
)

func main() {
	cfg := sink.ConfigFromEnv()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s, err := sink.New(cfg)
	if err != nil {
		log.Fatalf("aik-audit: failed to create sink: %v", err)
	}

	log.Printf("aik-audit: starting audit sink (nats=%s, clickhouse=%s, s3=%s)",
		cfg.NATSUrl, cfg.ClickHouseDSN, cfg.S3Endpoint)

	if err := s.Run(ctx); err != nil {
		log.Fatalf("aik-audit: sink stopped with error: %v", err)
	}

	// Allow in-flight flushes to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		log.Printf("aik-audit: shutdown error: %v", err)
	}

	log.Println("aik-audit: stopped")
}

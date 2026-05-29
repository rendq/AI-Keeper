// Package main provides the aik-marketplace binary — a REST API registry
// for SkillListing resources. It supports CRUD operations, tenant-scoped
// (internal) and global-scoped (external) publishing, and search/filter
// by category, tags, rating, and full-text.
//
// Requirements: C5.1, C5.2
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := envOr("LISTEN_ADDR", ":8090")
	dsn := envOr("DATABASE_URL", "")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var store Store
	if dsn != "" {
		// TODO: wire PostgreSQL store when available
		store = NewMemoryStore()
	} else {
		store = NewMemoryStore()
	}

	srv := NewServer(store)

	log.Printf("aik-marketplace: listening on %s", addr)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(addr)
	}()

	select {
	case <-ctx.Done():
		log.Println("aik-marketplace: shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("aik-marketplace: shutdown error: %v", err)
		}
	case err := <-errCh:
		if err != nil {
			log.Fatalf("aik-marketplace: server error: %v", err)
		}
	}

	log.Println("aik-marketplace: stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

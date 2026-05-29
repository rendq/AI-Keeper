// Package main implements the AIP PDP (Policy Decision Point) binary.
// It embeds OPA for Rego evaluation, exposes a gRPC Decide() RPC,
// and accepts bundle uploads via HTTP PUT for hot-loading.
//
// Requirements: A5.6, A5.7, A5.13, A5.10 (drift), B2.1, B2.2, B2.8, F6
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	aipv1 "github.com/ai-keeper/ai-keeper/proto/aip/v1"
)

const (
	defaultGRPCPort = "9090"
	defaultHTTPPort = "9091"
)

func main() {
	grpcPort := envOrDefault("PDP_GRPC_PORT", defaultGRPCPort)
	httpPort := envOrDefault("PDP_HTTP_PORT", defaultHTTPPort)

	pdpServer := NewPDPServer()

	// Start gRPC server.
	grpcLis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on gRPC port %s: %v", grpcPort, err)
	}
	grpcSrv := grpc.NewServer()
	aipv1.RegisterPolicyDecisionServiceServer(grpcSrv, pdpServer)

	go func() {
		log.Printf("PDP gRPC server listening on :%s", grpcPort)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC serve error: %v", err)
		}
	}()

	// Start HTTP server for bundle upload.
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/bundle", pdpServer.HandleBundleUpload)
	mux.HandleFunc("GET /v1/status", pdpServer.HandleStatus)

	httpSrv := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("PDP HTTP server listening on :%s", httpPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP serve error: %v", err)
		}
	}()

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("shutting down PDP...")
	grpcSrv.GracefulStop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
	fmt.Println("PDP stopped")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

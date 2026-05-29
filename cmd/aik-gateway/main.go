package gateway

import (
	"log"
	"net/http"
)

// Run starts the AIP Gateway HTTP server on the given address.
// This is the entry point for the gateway binary.
func Run(addr string, opts ...Option) error {
	gw := NewGateway(opts...)

	mux := http.NewServeMux()
	mux.Handle("/", gw)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("aik-gateway listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

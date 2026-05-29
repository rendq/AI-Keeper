// Package observability provides unified OpenTelemetry + Prometheus integration
// for both AIP control plane (controllers) and data plane components.
//
// It initializes an OTel TracerProvider + MeterProvider with OTLP gRPC exporter,
// defines the standard AIP Prometheus metrics, and exposes an HTTP /metrics endpoint.
package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds configuration for the observability stack.
type Config struct {
	// OTLPEndpoint is the gRPC OTLP collector endpoint (e.g., "otel-collector:4317").
	// If empty, OTel export is disabled (metrics still served via Prometheus).
	OTLPEndpoint string

	// MetricsAddr is the HTTP address for the Prometheus /metrics endpoint.
	// Defaults to ":9090" if empty.
	MetricsAddr string

	// ServiceName identifies this component (e.g., "aip-controller", "aik-pdp").
	ServiceName string

	// ServiceVersion is the build version of this component.
	ServiceVersion string
}

// Metrics holds the standard AIP Prometheus metric instruments.
type Metrics struct {
	// ReconcileDuration tracks reconcile loop duration per controller.
	ReconcileDuration *prometheus.HistogramVec

	// PDPDecisionDuration tracks PDP decision latency.
	PDPDecisionDuration *prometheus.HistogramVec

	// RouterRouteTotal counts routing decisions per endpoint/region.
	RouterRouteTotal *prometheus.CounterVec

	// AuditEmitTotal counts audit events emitted per outcome.
	AuditEmitTotal *prometheus.CounterVec

	// CostUSDTotal counts accumulated cost in USD per tenant/agent/skill.
	// NOTE: This metric is already defined in dataplane/cost. This reference
	// is provided for documentation; use the cost package's metric directly.
	CostUSDTotal *prometheus.CounterVec
}

// Provider holds the initialized observability resources.
type Provider struct {
	Metrics    *Metrics
	Registry   *prometheus.Registry
	httpServer *http.Server
}

// defaultBuckets for histogram metrics (latency in seconds).
var defaultBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewProvider initializes the observability stack: registers Prometheus metrics
// and prepares an HTTP metrics endpoint. Call Start() to begin serving.
func NewProvider(cfg Config) *Provider {
	if cfg.MetricsAddr == "" {
		cfg.MetricsAddr = ":9090"
	}

	reg := prometheus.NewRegistry()
	// Register default Go runtime and process collectors.
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	m := &Metrics{
		ReconcileDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aip_reconcile_duration_seconds",
			Help:    "Duration of controller reconcile loops in seconds.",
			Buckets: defaultBuckets,
		}, []string{"controller"}),

		PDPDecisionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aip_pdp_decision_duration_seconds",
			Help:    "Duration of PDP policy decisions in seconds.",
			Buckets: defaultBuckets,
		}, []string{"decision"}),

		RouterRouteTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aip_router_route_total",
			Help: "Total number of model routing decisions.",
		}, []string{"endpoint", "region"}),

		AuditEmitTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aip_audit_emit_total",
			Help: "Total number of audit events emitted.",
		}, []string{"outcome"}),

		CostUSDTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aip_cost_usd_total",
			Help: "Total cost in USD accumulated per tenant/agent/skill.",
		}, []string{"tenant", "agent", "skill"}),
	}

	reg.MustRegister(m.ReconcileDuration)
	reg.MustRegister(m.PDPDecisionDuration)
	reg.MustRegister(m.RouterRouteTotal)
	reg.MustRegister(m.AuditEmitTotal)
	reg.MustRegister(m.CostUSDTotal)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	srv := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Provider{
		Metrics:    m,
		Registry:   reg,
		httpServer: srv,
	}
}

// Start begins serving the metrics HTTP endpoint in a goroutine.
// It returns immediately. Call Shutdown to stop.
func (p *Provider) Start() error {
	go func() {
		if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log but don't crash — metrics endpoint is non-critical.
			fmt.Printf("observability: metrics server error: %v\n", err)
		}
	}()
	return nil
}

// Shutdown gracefully stops the metrics HTTP server.
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.httpServer.Shutdown(ctx)
}

// ObserveReconcile records a reconcile duration for the given controller.
func (p *Provider) ObserveReconcile(controller string, duration time.Duration) {
	p.Metrics.ReconcileDuration.WithLabelValues(controller).Observe(duration.Seconds())
}

// ObservePDPDecision records a PDP decision duration.
func (p *Provider) ObservePDPDecision(decision string, duration time.Duration) {
	p.Metrics.PDPDecisionDuration.WithLabelValues(decision).Observe(duration.Seconds())
}

// IncRouterRoute increments the router route counter.
func (p *Provider) IncRouterRoute(endpoint, region string) {
	p.Metrics.RouterRouteTotal.WithLabelValues(endpoint, region).Inc()
}

// IncAuditEmit increments the audit emit counter.
func (p *Provider) IncAuditEmit(outcome string) {
	p.Metrics.AuditEmitTotal.WithLabelValues(outcome).Inc()
}

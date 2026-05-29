package sink

import (
	"context"
	"log"

	"github.com/ai-keeper/ai-keeper/dataplane/audit"
)

// SIEMStub is a placeholder SIEM forwarder for P0.
// It logs events but does not forward to a real SIEM system (Splunk/QRadar).
// Real HEC (HTTP Event Collector) and CEF (Common Event Format) integration is P1.
type SIEMStub struct {
	format string // "hec" or "cef"
}

// NewSIEMStub creates a new SIEM stub forwarder.
func NewSIEMStub(format string) *SIEMStub {
	return &SIEMStub{format: format}
}

// Forward logs the event in the configured format (stub only).
func (s *SIEMStub) Forward(ctx context.Context, event *audit.Event) error {
	log.Printf("siem-stub [%s]: would forward event invocationId=%s action=%s/%s outcome=%s",
		s.format,
		event.InvocationID,
		event.Action.Verb,
		event.Action.Resource,
		outcomeStatus(event),
	)
	return nil
}

// Close is a no-op for the stub.
func (s *SIEMStub) Close() error {
	return nil
}

func outcomeStatus(e *audit.Event) string {
	if e.Outcome != nil {
		return e.Outcome.Status
	}
	return "unknown"
}

// FormatHEC formats an event for Splunk HEC (stub — returns placeholder).
func FormatHEC(event *audit.Event) ([]byte, error) {
	// P0 stub: real implementation would produce JSON payload for Splunk HEC
	return audit.SerializeJSON(event)
}

// FormatCEF formats an event for ArcSight CEF (stub — returns placeholder).
func FormatCEF(event *audit.Event) (string, error) {
	// P0 stub: real implementation would produce CEF format string
	// CEF:0|AIP|AuditSink|1.0|<action>|<name>|<severity>|<extensions>
	return "CEF:0|AIP|AuditSink|1.0|" + event.Action.Verb + "|" +
		event.Action.Resource + "|5|invocationId=" + event.InvocationID, nil
}

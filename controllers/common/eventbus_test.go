package common_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

func mustRef(t *testing.T, scheme sharedv1alpha1.ResourceRefScheme, path, version string) sharedv1alpha1.ResourceRef {
	t.Helper()
	r, err := sharedv1alpha1.FormatResourceRef(scheme, path, version)
	if err != nil {
		t.Fatalf("FormatResourceRef: %v", err)
	}
	return r
}

func TestSubjectFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		prefix string
		kind   common.DomainEventKind
		want   string
	}{
		{"", common.EventSkillPromoted, "aip.events.skillpromoted"},
		{"aip.events", common.EventPolicyDistributed, "aip.events.policydistributed"},
		{"my.prefix", common.EventAgentDeployed, "my.prefix.agentdeployed"},
	}
	for _, tc := range cases {
		got := common.SubjectFor(tc.prefix, tc.kind)
		if got != tc.want {
			t.Errorf("SubjectFor(%q, %q) = %q, want %q", tc.prefix, tc.kind, got, tc.want)
		}
	}
}

func TestNoopBus_PublishRecords(t *testing.T) {
	t.Parallel()

	bus := common.NewNoopBus(logr.Discard())
	ev := common.DomainEvent{
		Kind:    common.EventSkillPromoted,
		Subject: mustRef(t, sharedv1alpha1.SchemeSkill, "legal/contract-review", "1.0.0"),
		Payload: map[string]string{
			"from": "beta",
			"to":   "stable",
		},
		TraceID: "trace-abc",
	}
	if err := bus.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got := bus.Events()
	if len(got) != 1 {
		t.Fatalf("Events() = %d, want 1", len(got))
	}
	if got[0].Kind != common.EventSkillPromoted {
		t.Errorf("kind = %q", got[0].Kind)
	}
	if got[0].Timestamp.IsZero() {
		t.Error("Publish must populate Timestamp when zero")
	}
	if got[0].Payload["to"] != "stable" {
		t.Errorf("payload not preserved: %+v", got[0].Payload)
	}
}

func TestNoopBus_RejectsInvalidEvents(t *testing.T) {
	t.Parallel()

	bus := common.NewNoopBus(logr.Discard())
	ctx := context.Background()

	// Missing kind.
	err := bus.Publish(ctx, common.DomainEvent{
		Subject: mustRef(t, sharedv1alpha1.SchemeSkill, "x/y", ""),
	})
	if !errors.Is(err, common.ErrInvalidDomainEvent) {
		t.Errorf("missing kind: want ErrInvalidDomainEvent, got %v", err)
	}

	// Missing subject.
	err = bus.Publish(ctx, common.DomainEvent{
		Kind: common.EventAgentDeployed,
	})
	if !errors.Is(err, common.ErrInvalidDomainEvent) {
		t.Errorf("missing subject: want ErrInvalidDomainEvent, got %v", err)
	}

	// Malformed subject.
	err = bus.Publish(ctx, common.DomainEvent{
		Kind:    common.EventAgentDeployed,
		Subject: sharedv1alpha1.ResourceRef("not-a-ref"),
	})
	if !errors.Is(err, common.ErrInvalidDomainEvent) {
		t.Errorf("malformed subject: want ErrInvalidDomainEvent, got %v", err)
	}
}

func TestNoopBus_CloseStopsPublish(t *testing.T) {
	t.Parallel()

	bus := common.NewNoopBus(logr.Discard())
	if err := bus.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := bus.Publish(context.Background(), common.DomainEvent{
		Kind:    common.EventAgentDeployed,
		Subject: mustRef(t, sharedv1alpha1.SchemeAgent, "tenant/legal-copilot", ""),
	})
	if !errors.Is(err, common.ErrEventBusClosed) {
		t.Errorf("Publish after Close: want ErrEventBusClosed, got %v", err)
	}
}

func TestNoopBus_Reset(t *testing.T) {
	t.Parallel()

	bus := common.NewNoopBus(logr.Discard())
	for i := 0; i < 3; i++ {
		err := bus.Publish(context.Background(), common.DomainEvent{
			Kind:      common.EventPolicyDistributed,
			Subject:   mustRef(t, sharedv1alpha1.SchemePolicy, "tenant/p", ""),
			Timestamp: time.Now(),
		})
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	bus.Reset()
	if len(bus.Events()) != 0 {
		t.Fatalf("expected events to be cleared, got %d", len(bus.Events()))
	}
}

func TestAllDomainEventKinds_Include5MandatoryEvents(t *testing.T) {
	t.Parallel()

	mandatory := []common.DomainEventKind{
		common.EventSkillPromoted,
		common.EventSkillDeprecated,
		common.EventPolicyDistributed,
		common.EventAgentDeployed,
		common.EventAgentRolledBack,
	}
	known := map[common.DomainEventKind]bool{}
	for _, k := range common.AllDomainEventKinds {
		known[k] = true
	}
	for _, k := range mandatory {
		if !known[k] {
			t.Errorf("AllDomainEventKinds missing required kind %q", k)
		}
	}
}

func TestNATSJetStreamBus_RejectsEmptyURL(t *testing.T) {
	t.Parallel()

	if _, err := common.NewNATSJetStreamBus(""); err == nil {
		t.Fatal("expected error for empty NATS URL")
	}
}

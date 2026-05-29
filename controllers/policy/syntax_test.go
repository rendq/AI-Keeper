package policy

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// silence the apiextensions import if unused at link time.
var _ = apiextensionsv1.JSON{}

func TestValidateCEL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"simple equality", `principal.team == "research"`, false},
		{"complex with calls", `time.now() < timestamp("2026-12-31T00:00:00Z") && size(input.tools) >= 1`, false},
		{"invalid token soup", `not && a valid && expression =====`, true},
		{"unbalanced parens", `(a == 1`, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCEL(tc.expr)
			if tc.wantErr && err == nil {
				t.Fatalf("validateCEL(%q) = nil, want error", tc.expr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateCEL(%q) = %v, want nil", tc.expr, err)
			}
		})
	}
}

func TestValidateCron(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"5-field standard", "0 9 * * 1-5", false},
		{"6-field with seconds", "0 0 9 * * 1-5", false},
		{"@every shortcut", "@every 5m", false},
		{"@hourly", "@hourly", false},
		{"garbage", "not a cron", true},
		{"too many fields", "* * * * * * * * *", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCron(tc.expr)
			if tc.wantErr && err == nil {
				t.Fatalf("validateCron(%q) = nil, want error", tc.expr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateCron(%q) = %v, want nil", tc.expr, err)
			}
		})
	}
}

func TestValidateCIDR(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"ipv4 /24", "10.0.0.0/24", false},
		{"ipv6 /64", "fd00::/64", false},
		{"missing prefix", "10.0.0.0", true},
		{"bad prefix length", "10.0.0.0/64", true},
		{"junk", "not-an-ip", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCIDR(tc.expr)
			if tc.wantErr && err == nil {
				t.Fatalf("validateCIDR(%q) = nil, want error", tc.expr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateCIDR(%q) = %v, want nil", tc.expr, err)
			}
		})
	}
}

// TestValidatePolicy_AggregatesAllErrors verifies validatePolicy walks
// every annotated field and surfaces all violations at once.
func TestValidatePolicy_AggregatesAllErrors(t *testing.T) {
	t.Parallel()
	pol := &policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "agg", Namespace: "default"},
		Spec: policyv1alpha1.PolicySpec{
			Effect: "deny",
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{{Kind: "User"}},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{{Kind: "Skill"}},
				},
			},
			Conditions: &policyv1alpha1.ConditionSet{
				AllOf: []policyv1alpha1.ConditionItem{{
					Expression: "garbage && unbalanced ((",
					TimeWindow: &policyv1alpha1.ConditionTimeWindow{Schedule: "definitely not cron"},
					Location: &policyv1alpha1.ConditionLocation{
						IPAllowList: []string{"not-an-ip"},
						IPDenyList:  []string{"10.0.0.0/64"},
					},
				}},
			},
			Approvals: []policyv1alpha1.ApprovalSpec{{
				When: policyv1alpha1.ApprovalWhen{Expression: "((("},
				Approver: policyv1alpha1.ApprovalApprover{
					Kind: "User",
					Name: "alice",
				},
			}},
			Obligations: &policyv1alpha1.PolicyObligations{
				Notify: &policyv1alpha1.ObligationNotify{
					OnMatch: []policyv1alpha1.ObligationNotifyMatch{{
						Condition: "still ((( bad",
						Channel:   "ops",
					}},
				},
			},
		},
	}

	errs := validatePolicy(pol)
	if len(errs) < 6 {
		// 1 CEL conditions, 1 cron, 2 CIDR, 1 approval CEL, 1 notify CEL
		t.Fatalf("expected at least 6 errors, got %d: %v", len(errs), errs)
	}

	// Spot-check the field paths are sensible.
	joined := errs.ToAggregate().Error()
	for _, fragment := range []string{
		"spec.conditions.allOf[0].expression",
		"spec.conditions.allOf[0].timeWindow.schedule",
		"spec.conditions.allOf[0].location.ipAllowList[0]",
		"spec.conditions.allOf[0].location.ipDenyList[0]",
		"spec.approvals[0].when.expression",
		"spec.obligations.notify.onMatch[0].condition",
	} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("aggregated errors missing path %q: %s", fragment, joined)
		}
	}
}

// TestValidatePolicy_AcceptsValid returns no errors for a clean policy.
func TestValidatePolicy_AcceptsValid(t *testing.T) {
	t.Parallel()
	pol := &policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "clean", Namespace: "default"},
		Spec: policyv1alpha1.PolicySpec{
			Effect: "deny",
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{{Kind: "User"}},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{{Kind: "Skill"}},
				},
			},
			Conditions: &policyv1alpha1.ConditionSet{
				AllOf: []policyv1alpha1.ConditionItem{{
					Expression: `principal.team == "ops"`,
					TimeWindow: &policyv1alpha1.ConditionTimeWindow{Schedule: "0 9 * * 1-5"},
					Location: &policyv1alpha1.ConditionLocation{
						IPAllowList: []string{"10.0.0.0/8"},
						IPDenyList:  []string{"192.168.0.0/16"},
					},
				}},
			},
		},
	}
	if errs := validatePolicy(pol); len(errs) != 0 {
		t.Fatalf("validatePolicy returned errors on clean input: %v", errs)
	}
}

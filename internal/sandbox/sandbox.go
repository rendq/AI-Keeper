// Package sandbox provides helpers to inject RuntimeClass, security context,
// and generate NetworkPolicy resources for Agent sandbox isolation.
package sandbox

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// SandboxType enumerates supported sandbox runtimes.
type SandboxType string

const (
	SandboxGVisor      SandboxType = "gvisor"
	SandboxFirecracker SandboxType = "firecracker"
	SandboxKata        SandboxType = "kata"
	SandboxE2B         SandboxType = "e2b"
)

// NetworkPolicyMode enumerates sandbox network policy modes.
type NetworkPolicyMode string

const (
	NetworkPolicyDenyAll   NetworkPolicyMode = "deny_all"
	NetworkPolicyAllowList NetworkPolicyMode = "allow_list"
)

// SandboxConfig holds the configuration used to inject sandbox isolation
// into a Pod spec and generate associated NetworkPolicy resources.
type SandboxConfig struct {
	// Type selects the sandbox runtime (gvisor, firecracker, kata, e2b).
	Type SandboxType

	// NetworkPolicy selects the egress network policy mode.
	NetworkPolicy NetworkPolicyMode

	// EgressAllowList contains hostnames or CIDRs permitted when
	// NetworkPolicy is allow_list.
	EgressAllowList []string

	// CPULimit is the CPU resource limit (e.g. "2", "500m").
	CPULimit string

	// MemoryLimit is the memory resource limit (e.g. "512Mi", "2Gi").
	MemoryLimit string
}

// runtimeClassName returns the K8s RuntimeClass name for the sandbox type.
func runtimeClassName(t SandboxType) string {
	switch t {
	case SandboxGVisor:
		return "gvisor"
	case SandboxFirecracker:
		return "firecracker"
	case SandboxKata:
		return "kata"
	case SandboxE2B:
		return "e2b"
	default:
		return string(t)
	}
}

// InjectSandbox mutates the given PodSpec to add RuntimeClass, seccomp
// profile, and hardened security context based on the provided config.
//
// It sets:
//   - spec.runtimeClassName to the appropriate RuntimeClass
//   - pod-level seccomp profile (RuntimeDefault)
//   - for each container: readOnlyRootFilesystem, runAsNonRoot, drop ALL capabilities
func InjectSandbox(podSpec *corev1.PodSpec, config SandboxConfig) error {
	if podSpec == nil {
		return fmt.Errorf("podSpec must not be nil")
	}

	if config.Type == "" {
		return fmt.Errorf("sandbox type must be specified")
	}

	// Set RuntimeClassName
	rcName := runtimeClassName(config.Type)
	podSpec.RuntimeClassName = &rcName

	// Pod-level security context with seccomp profile
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{}
	}
	podSpec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}

	// Harden each container's security context
	for i := range podSpec.Containers {
		hardenContainer(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		hardenContainer(&podSpec.InitContainers[i])
	}

	return nil
}

// hardenContainer sets readOnlyRootFilesystem, runAsNonRoot, and drops ALL
// capabilities on the given container.
func hardenContainer(c *corev1.Container) {
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{}
	}
	c.SecurityContext.ReadOnlyRootFilesystem = ptr.To(true)
	c.SecurityContext.RunAsNonRoot = ptr.To(true)

	if c.SecurityContext.Capabilities == nil {
		c.SecurityContext.Capabilities = &corev1.Capabilities{}
	}
	c.SecurityContext.Capabilities.Drop = []corev1.Capability{"ALL"}
}

// GenerateNetworkPolicy creates a Kubernetes NetworkPolicy for the sandbox.
//
// When config.NetworkPolicy is deny_all, the policy denies all egress.
// When config.NetworkPolicy is allow_list, it allows egress only to the
// hosts/CIDRs in config.EgressAllowList.
func GenerateNetworkPolicy(namespace, agentName string, config SandboxConfig) *networkingv1.NetworkPolicy {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sandbox", agentName),
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "aip-agent-controller",
				"ai-keeper.io/agent":                agentName,
				"ai-keeper.io/component":            "sandbox",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"ai-keeper.io/agent":     agentName,
					"ai-keeper.io/component": "sandbox",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	switch config.NetworkPolicy {
	case NetworkPolicyDenyAll:
		// Empty Egress slice = deny all egress traffic
		np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{}

	case NetworkPolicyAllowList:
		rule := networkingv1.NetworkPolicyEgressRule{}
		for _, entry := range config.EgressAllowList {
			if isCIDR(entry) {
				rule.To = append(rule.To, networkingv1.NetworkPolicyPeer{
					IPBlock: &networkingv1.IPBlock{
						CIDR: entry,
					},
				})
			} else {
				// Hostname: resolve via DNS egress on port 53 + allow
				// the host as a CIDR /32 or /128 is not feasible at
				// NetworkPolicy level. Instead we allow DNS egress and
				// document that hostname-based filtering requires a CNI
				// that supports FQDN policies (e.g. Cilium).
				// For basic K8s NetworkPolicy, we allow all egress to
				// port 443 for listed hostnames (best-effort).
				rule.To = append(rule.To, networkingv1.NetworkPolicyPeer{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "0.0.0.0/0",
					},
				})
				// Add port 443 for HTTPS
				port443 := intstr.FromInt32(443)
				tcp := corev1.ProtocolTCP
				rule.Ports = append(rule.Ports, networkingv1.NetworkPolicyPort{
					Protocol: &tcp,
					Port:     &port443,
				})
			}
		}

		// Always allow DNS resolution (port 53 UDP)
		dnsRule := networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: ptr.To(corev1.ProtocolUDP),
					Port:     ptr.To(intstr.FromInt32(53)),
				},
			},
		}

		np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{dnsRule}
		if len(rule.To) > 0 || len(rule.Ports) > 0 {
			np.Spec.Egress = append(np.Spec.Egress, rule)
		}

	default:
		// No specific egress rules (deny all by having PolicyType Egress with empty rules)
		np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{}
	}

	return np
}

// isCIDR returns true if s looks like a CIDR notation (contains a slash).
func isCIDR(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

package sandbox

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func TestInjectSandbox_RuntimeClass(t *testing.T) {
	tests := []struct {
		name         string
		sandboxType  SandboxType
		wantRCName   string
	}{
		{"gvisor", SandboxGVisor, "gvisor"},
		{"firecracker", SandboxFirecracker, "firecracker"},
		{"kata", SandboxKata, "kata"},
		{"e2b", SandboxE2B, "e2b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app"}},
			}
			config := SandboxConfig{Type: tt.sandboxType}

			if err := InjectSandbox(podSpec, config); err != nil {
				t.Fatalf("InjectSandbox() error = %v", err)
			}

			if podSpec.RuntimeClassName == nil {
				t.Fatal("RuntimeClassName is nil")
			}
			if *podSpec.RuntimeClassName != tt.wantRCName {
				t.Errorf("RuntimeClassName = %q, want %q", *podSpec.RuntimeClassName, tt.wantRCName)
			}
		})
	}
}

func TestInjectSandbox_SecurityContext(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
			{Name: "sidecar"},
		},
		InitContainers: []corev1.Container{
			{Name: "init"},
		},
	}
	config := SandboxConfig{Type: SandboxGVisor}

	if err := InjectSandbox(podSpec, config); err != nil {
		t.Fatalf("InjectSandbox() error = %v", err)
	}

	// Check pod-level seccomp profile
	if podSpec.SecurityContext == nil {
		t.Fatal("pod SecurityContext is nil")
	}
	if podSpec.SecurityContext.SeccompProfile == nil {
		t.Fatal("SeccompProfile is nil")
	}
	if podSpec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("SeccompProfile.Type = %v, want RuntimeDefault", podSpec.SecurityContext.SeccompProfile.Type)
	}

	// Check all containers
	allContainers := append(podSpec.Containers, podSpec.InitContainers...)
	for _, c := range allContainers {
		t.Run(c.Name, func(t *testing.T) {
			sc := c.SecurityContext
			if sc == nil {
				t.Fatal("container SecurityContext is nil")
			}
			if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				t.Error("ReadOnlyRootFilesystem should be true")
			}
			if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				t.Error("RunAsNonRoot should be true")
			}
			if sc.Capabilities == nil {
				t.Fatal("Capabilities is nil")
			}
			if len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" {
				t.Errorf("Capabilities.Drop = %v, want [ALL]", sc.Capabilities.Drop)
			}
		})
	}
}

func TestInjectSandbox_NilPodSpec(t *testing.T) {
	err := InjectSandbox(nil, SandboxConfig{Type: SandboxGVisor})
	if err == nil {
		t.Error("expected error for nil podSpec")
	}
}

func TestInjectSandbox_EmptyType(t *testing.T) {
	podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}
	err := InjectSandbox(podSpec, SandboxConfig{})
	if err == nil {
		t.Error("expected error for empty sandbox type")
	}
}

func TestGenerateNetworkPolicy_DenyAll(t *testing.T) {
	config := SandboxConfig{
		Type:          SandboxGVisor,
		NetworkPolicy: NetworkPolicyDenyAll,
	}

	np := GenerateNetworkPolicy("test-ns", "my-agent", config)

	if np.Namespace != "test-ns" {
		t.Errorf("Namespace = %q, want %q", np.Namespace, "test-ns")
	}
	if np.Name != "my-agent-sandbox" {
		t.Errorf("Name = %q, want %q", np.Name, "my-agent-sandbox")
	}

	// Should have Egress policy type
	if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != networkingv1.PolicyTypeEgress {
		t.Errorf("PolicyTypes = %v, want [Egress]", np.Spec.PolicyTypes)
	}

	// deny_all = empty egress rules
	if len(np.Spec.Egress) != 0 {
		t.Errorf("Egress rules = %d, want 0 for deny_all", len(np.Spec.Egress))
	}

	// Check pod selector labels
	sel := np.Spec.PodSelector.MatchLabels
	if sel["ai-keeper.io/agent"] != "my-agent" {
		t.Errorf("PodSelector label ai-keeper.io/agent = %q, want %q", sel["ai-keeper.io/agent"], "my-agent")
	}
}

func TestGenerateNetworkPolicy_AllowList_CIDR(t *testing.T) {
	config := SandboxConfig{
		Type:            SandboxGVisor,
		NetworkPolicy:   NetworkPolicyAllowList,
		EgressAllowList: []string{"10.0.0.0/8", "192.168.1.0/24"},
	}

	np := GenerateNetworkPolicy("prod", "code-agent", config)

	// Should have DNS rule + CIDR rule
	if len(np.Spec.Egress) != 2 {
		t.Fatalf("Egress rules = %d, want 2", len(np.Spec.Egress))
	}

	// First rule should be DNS (port 53 UDP)
	dnsRule := np.Spec.Egress[0]
	if len(dnsRule.Ports) != 1 {
		t.Fatalf("DNS rule ports = %d, want 1", len(dnsRule.Ports))
	}
	if dnsRule.Ports[0].Port.IntValue() != 53 {
		t.Errorf("DNS port = %d, want 53", dnsRule.Ports[0].Port.IntValue())
	}

	// Second rule should have the CIDRs
	cidrRule := np.Spec.Egress[1]
	if len(cidrRule.To) != 2 {
		t.Fatalf("CIDR rule To = %d, want 2", len(cidrRule.To))
	}
	if cidrRule.To[0].IPBlock.CIDR != "10.0.0.0/8" {
		t.Errorf("CIDR[0] = %q, want %q", cidrRule.To[0].IPBlock.CIDR, "10.0.0.0/8")
	}
	if cidrRule.To[1].IPBlock.CIDR != "192.168.1.0/24" {
		t.Errorf("CIDR[1] = %q, want %q", cidrRule.To[1].IPBlock.CIDR, "192.168.1.0/24")
	}
}

func TestGenerateNetworkPolicy_AllowList_Hostname(t *testing.T) {
	config := SandboxConfig{
		Type:            SandboxFirecracker,
		NetworkPolicy:   NetworkPolicyAllowList,
		EgressAllowList: []string{"api.example.com"},
	}

	np := GenerateNetworkPolicy("default", "safe-agent", config)

	// Should have DNS rule + hostname rule
	if len(np.Spec.Egress) < 2 {
		t.Fatalf("Egress rules = %d, want at least 2", len(np.Spec.Egress))
	}

	// The hostname rule should use 0.0.0.0/0 with port 443
	hostRule := np.Spec.Egress[1]
	if len(hostRule.To) == 0 {
		t.Fatal("hostname rule has no To peers")
	}
	if hostRule.To[0].IPBlock.CIDR != "0.0.0.0/0" {
		t.Errorf("hostname CIDR = %q, want 0.0.0.0/0", hostRule.To[0].IPBlock.CIDR)
	}
	if len(hostRule.Ports) == 0 {
		t.Fatal("hostname rule has no ports")
	}
	if hostRule.Ports[0].Port.IntValue() != 443 {
		t.Errorf("hostname port = %d, want 443", hostRule.Ports[0].Port.IntValue())
	}
}

func TestGenerateNetworkPolicy_Labels(t *testing.T) {
	config := SandboxConfig{
		Type:          SandboxKata,
		NetworkPolicy: NetworkPolicyDenyAll,
	}

	np := GenerateNetworkPolicy("ns1", "agent-x", config)

	labels := np.Labels
	if labels["app.kubernetes.io/managed-by"] != "aip-agent-controller" {
		t.Errorf("managed-by label = %q", labels["app.kubernetes.io/managed-by"])
	}
	if labels["ai-keeper.io/agent"] != "agent-x" {
		t.Errorf("agent label = %q", labels["ai-keeper.io/agent"])
	}
	if labels["ai-keeper.io/component"] != "sandbox" {
		t.Errorf("component label = %q", labels["ai-keeper.io/component"])
	}
}

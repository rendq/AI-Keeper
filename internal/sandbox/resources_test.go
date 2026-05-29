package sandbox

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestResourceLimits_Injection(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
			{Name: "sidecar"},
		},
		InitContainers: []corev1.Container{
			{Name: "init"},
		},
	}

	err := InjectResourceLimits(podSpec, "2", "512Mi")
	if err != nil {
		t.Fatalf("InjectResourceLimits() error = %v", err)
	}

	expectedCPU := resource.MustParse("2")
	expectedMem := resource.MustParse("512Mi")

	// Check all containers (including init containers)
	allContainers := append(podSpec.Containers, podSpec.InitContainers...)
	for _, c := range allContainers {
		t.Run(c.Name, func(t *testing.T) {
			if c.Resources.Limits == nil {
				t.Fatal("Resources.Limits is nil")
			}
			cpuLim := c.Resources.Limits[corev1.ResourceCPU]
			if !cpuLim.Equal(expectedCPU) {
				t.Errorf("CPU limit = %v, want %v", cpuLim.String(), expectedCPU.String())
			}
			memLim := c.Resources.Limits[corev1.ResourceMemory]
			if !memLim.Equal(expectedMem) {
				t.Errorf("Memory limit = %v, want %v", memLim.String(), expectedMem.String())
			}
		})
	}
}

func TestResourceLimits_ParseQuantity(t *testing.T) {
	tests := []struct {
		name        string
		cpu         string
		mem         string
		wantErr     bool
	}{
		{"valid millicpu and Mi", "500m", "256Mi", false},
		{"valid whole cpu and Gi", "4", "2Gi", false},
		{"valid fractional cpu", "1.5", "1Gi", false},
		{"invalid cpu", "notacpu", "512Mi", true},
		{"invalid memory", "1", "notamem", true},
		{"empty cpu", "", "512Mi", true},
		{"empty memory", "1", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app"}},
			}
			err := InjectResourceLimits(podSpec, tt.cpu, tt.mem)
			if (err != nil) != tt.wantErr {
				t.Errorf("InjectResourceLimits() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResourceLimits_NilPodSpec(t *testing.T) {
	err := InjectResourceLimits(nil, "1", "512Mi")
	if err == nil {
		t.Error("expected error for nil podSpec")
	}
}

func TestEphemeralFS_Injection(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
			{Name: "sidecar"},
		},
		InitContainers: []corev1.Container{
			{Name: "init"},
		},
	}

	err := InjectEphemeralFS(podSpec)
	if err != nil {
		t.Fatalf("InjectEphemeralFS() error = %v", err)
	}

	// Verify volume was added with Memory medium
	if len(podSpec.Volumes) != 1 {
		t.Fatalf("Volumes count = %d, want 1", len(podSpec.Volumes))
	}
	vol := podSpec.Volumes[0]
	if vol.Name != ephemeralVolumeName {
		t.Errorf("Volume name = %q, want %q", vol.Name, ephemeralVolumeName)
	}
	if vol.EmptyDir == nil {
		t.Fatal("Volume.EmptyDir is nil")
	}
	if vol.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Errorf("EmptyDir.Medium = %q, want %q", vol.EmptyDir.Medium, corev1.StorageMediumMemory)
	}

	// Verify mount in all containers
	allContainers := append(podSpec.Containers, podSpec.InitContainers...)
	for _, c := range allContainers {
		t.Run(c.Name, func(t *testing.T) {
			found := false
			for _, m := range c.VolumeMounts {
				if m.Name == ephemeralVolumeName && m.MountPath == ephemeralMountPath {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("container %q missing ephemeral volume mount at %s", c.Name, ephemeralMountPath)
			}
		})
	}
}

func TestEphemeralFS_NilPodSpec(t *testing.T) {
	err := InjectEphemeralFS(nil)
	if err == nil {
		t.Error("expected error for nil podSpec")
	}
}

func TestSandboxEvent_Creation(t *testing.T) {
	t.Run("start event", func(t *testing.T) {
		before := time.Now()
		event := NewSandboxStartEvent("my-agent", SandboxGVisor)
		after := time.Now()

		if event.AgentName != "my-agent" {
			t.Errorf("AgentName = %q, want %q", event.AgentName, "my-agent")
		}
		if event.SandboxType != SandboxGVisor {
			t.Errorf("SandboxType = %q, want %q", event.SandboxType, SandboxGVisor)
		}
		if event.Action != SandboxActionStarted {
			t.Errorf("Action = %q, want %q", event.Action, SandboxActionStarted)
		}
		if event.Timestamp.Before(before) || event.Timestamp.After(after) {
			t.Error("Timestamp not within expected range")
		}
		if event.Duration != 0 {
			t.Errorf("Duration = %v, want 0", event.Duration)
		}
		if event.ExitCode != 0 {
			t.Errorf("ExitCode = %d, want 0", event.ExitCode)
		}
		if event.OOMKilled {
			t.Error("OOMKilled should be false for start event")
		}
	})

	t.Run("destroy event", func(t *testing.T) {
		dur := 30 * time.Second
		before := time.Now()
		event := NewSandboxDestroyEvent("code-runner", SandboxFirecracker, dur, 137, true)
		after := time.Now()

		if event.AgentName != "code-runner" {
			t.Errorf("AgentName = %q, want %q", event.AgentName, "code-runner")
		}
		if event.SandboxType != SandboxFirecracker {
			t.Errorf("SandboxType = %q, want %q", event.SandboxType, SandboxFirecracker)
		}
		if event.Action != SandboxActionDestroyed {
			t.Errorf("Action = %q, want %q", event.Action, SandboxActionDestroyed)
		}
		if event.Timestamp.Before(before) || event.Timestamp.After(after) {
			t.Error("Timestamp not within expected range")
		}
		if event.Duration != dur {
			t.Errorf("Duration = %v, want %v", event.Duration, dur)
		}
		if event.ExitCode != 137 {
			t.Errorf("ExitCode = %d, want 137", event.ExitCode)
		}
		if !event.OOMKilled {
			t.Error("OOMKilled should be true")
		}
	})
}

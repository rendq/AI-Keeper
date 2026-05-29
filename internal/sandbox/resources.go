package sandbox

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// InjectResourceLimits sets CPU and memory resource limits on all containers
// in the given PodSpec for strict cgroup enforcement. When limits are exceeded,
// the container will be OOM-killed (memory) or throttled (CPU).
func InjectResourceLimits(podSpec *corev1.PodSpec, cpuLimit, memoryLimit string) error {
	if podSpec == nil {
		return fmt.Errorf("podSpec must not be nil")
	}
	if cpuLimit == "" {
		return fmt.Errorf("cpuLimit must not be empty")
	}
	if memoryLimit == "" {
		return fmt.Errorf("memoryLimit must not be empty")
	}

	cpuQty, err := resource.ParseQuantity(cpuLimit)
	if err != nil {
		return fmt.Errorf("invalid cpuLimit %q: %w", cpuLimit, err)
	}
	memQty, err := resource.ParseQuantity(memoryLimit)
	if err != nil {
		return fmt.Errorf("invalid memoryLimit %q: %w", memoryLimit, err)
	}

	limits := corev1.ResourceList{
		corev1.ResourceCPU:    cpuQty,
		corev1.ResourceMemory: memQty,
	}

	for i := range podSpec.Containers {
		if podSpec.Containers[i].Resources.Limits == nil {
			podSpec.Containers[i].Resources.Limits = make(corev1.ResourceList)
		}
		podSpec.Containers[i].Resources.Limits[corev1.ResourceCPU] = limits[corev1.ResourceCPU]
		podSpec.Containers[i].Resources.Limits[corev1.ResourceMemory] = limits[corev1.ResourceMemory]
	}
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Resources.Limits == nil {
			podSpec.InitContainers[i].Resources.Limits = make(corev1.ResourceList)
		}
		podSpec.InitContainers[i].Resources.Limits[corev1.ResourceCPU] = limits[corev1.ResourceCPU]
		podSpec.InitContainers[i].Resources.Limits[corev1.ResourceMemory] = limits[corev1.ResourceMemory]
	}

	return nil
}

const (
	ephemeralVolumeName = "sandbox-ephemeral"
	ephemeralMountPath  = "/tmp"
)

// InjectEphemeralFS adds an emptyDir volume with medium:Memory to the PodSpec
// and mounts it at /tmp in all containers. This provides a fast, ephemeral
// filesystem that is automatically cleaned up when the pod terminates.
func InjectEphemeralFS(podSpec *corev1.PodSpec) error {
	if podSpec == nil {
		return fmt.Errorf("podSpec must not be nil")
	}

	// Add the emptyDir volume with Memory medium
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: ephemeralVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	})

	// Mount it in all containers
	mount := corev1.VolumeMount{
		Name:      ephemeralVolumeName,
		MountPath: ephemeralMountPath,
	}
	for i := range podSpec.Containers {
		podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, mount)
	}
	for i := range podSpec.InitContainers {
		podSpec.InitContainers[i].VolumeMounts = append(podSpec.InitContainers[i].VolumeMounts, mount)
	}

	return nil
}

// SandboxAction describes the lifecycle event of a sandbox.
type SandboxAction string

const (
	SandboxActionStarted   SandboxAction = "started"
	SandboxActionExecuted  SandboxAction = "executed"
	SandboxActionDestroyed SandboxAction = "destroyed"
)

// SandboxEvent represents an audit event for sandbox lifecycle operations.
type SandboxEvent struct {
	// AgentName is the name of the agent that owns the sandbox.
	AgentName string

	// SandboxType is the type of sandbox runtime used.
	SandboxType SandboxType

	// Action is the lifecycle action (started, executed, destroyed).
	Action SandboxAction

	// Timestamp is when the event occurred.
	Timestamp time.Time

	// Duration is how long the sandbox was alive (for destroy events).
	Duration time.Duration

	// ExitCode is the exit code of the sandbox process (for destroy events).
	ExitCode int

	// OOMKilled indicates whether the sandbox was killed due to OOM.
	OOMKilled bool
}

// NewSandboxStartEvent creates a new audit event for sandbox start.
func NewSandboxStartEvent(agentName string, sandboxType SandboxType) SandboxEvent {
	return SandboxEvent{
		AgentName:   agentName,
		SandboxType: sandboxType,
		Action:      SandboxActionStarted,
		Timestamp:   time.Now(),
	}
}

// NewSandboxDestroyEvent creates a new audit event for sandbox destruction.
func NewSandboxDestroyEvent(agentName string, sandboxType SandboxType, duration time.Duration, exitCode int, oomKilled bool) SandboxEvent {
	return SandboxEvent{
		AgentName:   agentName,
		SandboxType: sandboxType,
		Action:      SandboxActionDestroyed,
		Timestamp:   time.Now(),
		Duration:    duration,
		ExitCode:    exitCode,
		OOMKilled:   oomKilled,
	}
}

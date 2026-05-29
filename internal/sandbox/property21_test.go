//go:build pbt

package sandbox

import (
	"fmt"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	corev1 "k8s.io/api/core/v1"
)

// TestProperty21 validates that sandbox resource limits are strictly injected
// into all containers in the PodSpec.
//
// **Validates: Requirements B7.4**
//
// Property: For any random (cpuLimit, memoryLimit, sandboxType) combination,
// after InjectSandbox + InjectResourceLimits, every container has:
//   - resources.limits.cpu == config.CPULimit
//   - resources.limits.memory == config.MemoryLimit
//   - RuntimeClassName matches the sandbox type
//   - Security context hardening is always present
func TestProperty21(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 1000
	parameters.MaxSize = 100
	properties := gopter.NewProperties(parameters)

	sandboxTypes := []SandboxType{SandboxGVisor, SandboxFirecracker, SandboxKata, SandboxE2B}

	// Generator for sandbox type index
	genSandboxTypeIdx := gen.IntRange(0, len(sandboxTypes)-1)

	// Generator for CPU millicore values (100m to 8000m)
	genCPUMilli := gen.IntRange(100, 8000)

	// Generator for memory in Mi (64Mi to 8192Mi i.e. 8Gi)
	genMemoryMi := gen.IntRange(64, 8192)

	// Generator for number of containers (1 to 4)
	genNumContainers := gen.IntRange(1, 4)

	properties.Property("resource limits strictly match configuration", prop.ForAll(
		func(typeIdx, cpuMilli, memMi, numContainers int) bool {
			sandboxType := sandboxTypes[typeIdx]
			cpuLimit := fmt.Sprintf("%dm", cpuMilli)
			memoryLimit := fmt.Sprintf("%dMi", memMi)

			// Build a PodSpec with random number of containers
			containers := make([]corev1.Container, numContainers)
			for i := range containers {
				containers[i] = corev1.Container{Name: fmt.Sprintf("container-%d", i)}
			}
			podSpec := &corev1.PodSpec{
				Containers: containers,
			}

			config := SandboxConfig{
				Type:        sandboxType,
				CPULimit:    cpuLimit,
				MemoryLimit: memoryLimit,
			}

			// Apply sandbox injection
			if err := InjectSandbox(podSpec, config); err != nil {
				t.Logf("InjectSandbox failed: %v", err)
				return false
			}

			// Apply resource limits
			if err := InjectResourceLimits(podSpec, cpuLimit, memoryLimit); err != nil {
				t.Logf("InjectResourceLimits failed: %v", err)
				return false
			}

			// Assert: RuntimeClassName matches sandbox type
			expectedRC := runtimeClassName(sandboxType)
			if podSpec.RuntimeClassName == nil || *podSpec.RuntimeClassName != expectedRC {
				t.Logf("RuntimeClassName mismatch: got %v, want %s", podSpec.RuntimeClassName, expectedRC)
				return false
			}

			// Assert: every container has correct resource limits and security hardening
			for i, c := range podSpec.Containers {
				// Check CPU limit
				cpuQty := c.Resources.Limits[corev1.ResourceCPU]
				if cpuQty.String() != cpuLimit {
					t.Logf("container[%d] CPU limit: got %s, want %s", i, cpuQty.String(), cpuLimit)
					return false
				}

				// Check memory limit
				memQty := c.Resources.Limits[corev1.ResourceMemory]
				if memQty.String() != memoryLimit {
					t.Logf("container[%d] memory limit: got %s, want %s", i, memQty.String(), memoryLimit)
					return false
				}

				// Check security context hardening
				if c.SecurityContext == nil {
					t.Logf("container[%d] SecurityContext is nil", i)
					return false
				}
				if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
					t.Logf("container[%d] ReadOnlyRootFilesystem not set", i)
					return false
				}
				if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
					t.Logf("container[%d] RunAsNonRoot not set", i)
					return false
				}
				if c.SecurityContext.Capabilities == nil {
					t.Logf("container[%d] Capabilities is nil", i)
					return false
				}
				foundDropAll := false
				for _, cap := range c.SecurityContext.Capabilities.Drop {
					if cap == "ALL" {
						foundDropAll = true
						break
					}
				}
				if !foundDropAll {
					t.Logf("container[%d] does not drop ALL capabilities", i)
					return false
				}
			}

			return true
		},
		genSandboxTypeIdx,
		genCPUMilli,
		genMemoryMi,
		genNumContainers,
	))

	properties.TestingRun(t)
}

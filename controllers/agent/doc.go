// Package agent implements the AIP Agent controller (design.md §6.2).
//
// The reconciler converges an [agentv1alpha1.Agent] CR into a running
// Deployment + ServiceAccount + Channel webhook stack, executing the
// state machine documented in design.md §6.2.1 and the deletion flow
// in design.md §6.2.4.
//
// Scope (P0 MVP — task 3.4):
//
//   - Only `runtime.pattern ∈ {react, tool_calling}` is accepted; all
//     other values flip `SpecValid=False reason=UnsupportedPattern` and
//     drive `phase=Failed`.
//   - The RolloutController grey path described in design.md §6.2.3 is
//     intentionally skipped — a successful Agent gets 100 % traffic
//     immediately (`RolloutComplete=True`, `RolloutStatus.Phase=Succeeded`).
//   - Cross-controller pluggables (Policy binder, Identity provisioner,
//     Channel registrar, Deployment manager) are interface-typed; the
//     real implementations land in tasks 5.x / 6.x / 14.x. This package
//     ships [Noop*] stand-ins that keep the reconciler driveable in
//     unit tests.
//
// Conditions emitted (design.md §6.2.5 / Requirements A4.1, A4.2,
// A4.6, A4.7, A4.8, A4.9, A4.10, A6.1, A6.2):
//
//   - `SpecValid` — `runtime.pattern` is supported in P0
//   - `SkillsResolved` — every entry in `spec.skills[]` resolves to a
//     concrete `Skill@version` per [internal/resolver.Constraint]
//   - `PolicyAttached` — `PolicyBinder.Bind` succeeded
//   - `IdentityReady` — `IdentityProvisioner.Provision` succeeded
//   - `Deployed` — `DeploymentManager.EnsureDeployment` reports
//     replicas == desired
//   - `ChannelsHealthy` — `ChannelRegistrar.RegisterChannels` succeeded
//   - `GuardrailsHealthy` — defaulted to True for P0 (no guardrail
//     wiring in this build)
//   - `SandboxReady` — when sandbox is enabled, the requested
//     RuntimeClass is present in the cluster
//   - `RolloutComplete` — set True alongside `Deployed=True`
//   - `BudgetWithinLimit` — defaulted to True for P0
//   - `UsingDeprecatedSkill` — flipped True when any resolved Skill
//     carries `Deprecating=True`
//   - `Ready` — aggregate of the gates above per design.md §6.2.5
//
// Validates: Requirements A4.1, A4.2, A4.6, A4.7, A4.8, A4.9, A4.10,
// A6.1, A6.2.
package agent

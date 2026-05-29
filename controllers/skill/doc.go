// Package skill implements the AIP Skill controller.
//
// The reconciler implements the state machine described in
// design.md §6.1 (Pending / Validating / Resolving / Building /
// Registering / Evaluating / Active / Degraded / Failed / Deprecated /
// Terminating). It is the component that takes a `Skill` CR through
// schema validation, dependency resolution, implementation readiness
// check and registration into the Skill_Registry.
//
// The package exposes three small interfaces so the data-plane
// dependencies — JSON Schema validation, dependency resolution and the
// Skill_Registry — can be plugged in without coupling the controller to
// any one implementation:
//
//   - [SchemaValidator]: validates the JSON Schema embedded in
//     `spec.interface.input.schema` and `spec.interface.output.schema`.
//     The default implementation lives in `jsonschema.go` and uses
//     santhosh-tekuri/jsonschema/v5.
//
//   - [Resolver]: resolves `spec.implementation.requires` against the
//     live cluster state. The default `NoopResolver` always succeeds; the
//     real implementation lands in task 3.3 under `internal/resolver/`.
//
//   - [Registry]: persists Skill@version into the Skill_Registry. The
//     default in-memory implementation [MemoryRegistry] is used in P0
//     until the registry service from task 16.1 is wired up.
//
// Validates: Requirements A3.1—A3.7, A3.11—A3.13, A6.4.
package skill

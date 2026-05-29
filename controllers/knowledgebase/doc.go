// Package knowledgebase implements the AIP KnowledgeBase controller
// (design.md §6.5 — KnowledgeBase Controller / Requirement A7.5).
//
// The reconciler turns a [datav1alpha1.KnowledgeBase] CR into an
// indexed retrieval-ready aggregation of DataSources. P0 ships a
// placeholder pipeline — the full chunking → embedding → enrichment
// flow lands in P1. The controller still:
//
//   - validates that every referenced DataSource exists in the same
//     namespace (Requirement A7.5),
//   - enforces the lint rule `kb/acl-not-open` at runtime so a KB
//     classified ≥ confidential cannot run with `acl.mode=open`
//     (Requirement A9.2),
//   - drives the [Pipeline] interface to populate `status.{chunkCount,
//     indexSizeBytes, lastIndexedAt}` so dashboards have non-zero data
//     to render.
//
// Conditions emitted (design.md §6.5 / Requirement A7.5):
//
//   - `SourcesReady` — every referenced DataSource exists.
//   - `Indexed` — pipeline reported a non-zero chunk count.
//   - `Synced` — pipeline reported a recent `lastIndexedAt`.
//   - `Ready` — aggregate of the gates above.
//
// Deletion:
//
//   - The reconciler adds the `ai-keeper.io/kb-protect` finalizer so a future
//     P1 index drainer has a safe hook before the CR disappears.
//
// Validates: Requirements A7.5 (basic).
package knowledgebase

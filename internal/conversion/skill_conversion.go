package conversion

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	skillv1beta1 "github.com/ai-keeper/ai-keeper/api/skill/v1beta1"
)

// ConvertSkillAlphaToBeta converts a v1alpha1.Skill to v1beta1.Skill.
// The returned []string lists lossy annotations (fields in beta that
// have no alpha equivalent and thus remain empty after up-conversion).
//
// Validates: Requirements A11.2, A11.3, A11.4.
func ConvertSkillAlphaToBeta(src *skillv1alpha1.Skill) (*skillv1beta1.Skill, []string) {
	if src == nil {
		return nil, nil
	}

	dst := &skillv1beta1.Skill{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "skill.ai-keeper.io/v1beta1",
			Kind:       "Skill",
		},
		ObjectMeta: *src.ObjectMeta.DeepCopy(),
	}

	// Spec — shared fields map directly.
	dst.Spec.Version = src.Spec.Version
	dst.Spec.Stability = src.Spec.Stability
	dst.Spec.Interface = convertSkillInterfaceAlphaToBeta(src.Spec.Interface)
	dst.Spec.Implementation = convertSkillImplementationAlphaToBeta(src.Spec.Implementation)

	if src.Spec.Governance != nil {
		g := *src.Spec.Governance
		dst.Spec.Governance = &g
	}
	if src.Spec.Cost != nil {
		c := *src.Spec.Cost
		dst.Spec.Cost = &c
	}
	if src.Spec.SLO != nil {
		s := *src.Spec.SLO
		dst.Spec.SLO = &s
	}
	if src.Spec.Reliability != nil {
		r := *src.Spec.Reliability
		dst.Spec.Reliability = &r
	}
	if src.Spec.Evaluation != nil {
		dst.Spec.Evaluation = convertSkillEvalAlphaToBeta(src.Spec.Evaluation)
	}
	if src.Spec.Lifecycle != nil {
		dst.Spec.Lifecycle = convertSkillLifecycleAlphaToBeta(src.Spec.Lifecycle)
	}
	// Compliance is new in v1beta1 — no alpha source, stays nil.

	// Status
	dst.Status = convertSkillStatusAlphaToBeta(src.Status)

	// No lossy annotations for alpha→beta: beta is a superset.
	return dst, nil
}

// ConvertSkillBetaToAlpha converts a v1beta1.Skill to v1alpha1.Skill.
// Fields only present in v1beta1 (compliance, eval metrics,
// continuousEval) are lost — returned as lossy annotation strings.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func ConvertSkillBetaToAlpha(src *skillv1beta1.Skill) (*skillv1alpha1.Skill, []string) {
	if src == nil {
		return nil, nil
	}

	var lossy []string

	dst := &skillv1alpha1.Skill{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "skill.ai-keeper.io/v1alpha1",
			Kind:       "Skill",
		},
		ObjectMeta: *src.ObjectMeta.DeepCopy(),
	}

	// Spec — shared fields.
	dst.Spec.Version = src.Spec.Version
	dst.Spec.Stability = src.Spec.Stability
	dst.Spec.Interface = convertSkillInterfaceBetaToAlpha(src.Spec.Interface)
	dst.Spec.Implementation = convertSkillImplementationBetaToAlpha(src.Spec.Implementation)

	if src.Spec.Governance != nil {
		g := *src.Spec.Governance
		dst.Spec.Governance = &g
	}
	if src.Spec.Cost != nil {
		c := *src.Spec.Cost
		dst.Spec.Cost = &c
	}
	if src.Spec.SLO != nil {
		s := *src.Spec.SLO
		dst.Spec.SLO = &s
	}
	if src.Spec.Reliability != nil {
		r := *src.Spec.Reliability
		dst.Spec.Reliability = &r
	}
	if src.Spec.Evaluation != nil {
		dst.Spec.Evaluation, lossy = convertSkillEvalBetaToAlpha(src.Spec.Evaluation, lossy)
	}
	if src.Spec.Lifecycle != nil {
		dst.Spec.Lifecycle = convertSkillLifecycleBetaToAlpha(src.Spec.Lifecycle)
	}

	// Compliance block is lossy — no v1alpha1 equivalent.
	if src.Spec.Compliance != nil {
		lossy = append(lossy, "v1beta1→v1alpha1: spec.compliance dropped (no v1alpha1 equivalent)")
	}

	// Status
	dst.Status = convertSkillStatusBetaToAlpha(src.Status)

	return dst, lossy
}

// --- Helper converters ---

func convertSkillInterfaceAlphaToBeta(in skillv1alpha1.SkillInterface) skillv1beta1.SkillInterface {
	out := skillv1beta1.SkillInterface{
		Input:  skillv1beta1.SkillIO{Schema: in.Input.Schema},
		Output: skillv1beta1.SkillIO{Schema: in.Output.Schema},
	}
	for _, ex := range in.Examples {
		out.Examples = append(out.Examples, skillv1beta1.SkillExample{
			Note:   ex.Note,
			Input:  ex.Input,
			Output: ex.Output,
		})
	}
	return out
}

func convertSkillInterfaceBetaToAlpha(in skillv1beta1.SkillInterface) skillv1alpha1.SkillInterface {
	out := skillv1alpha1.SkillInterface{
		Input:  skillv1alpha1.SkillIO{Schema: in.Input.Schema},
		Output: skillv1alpha1.SkillIO{Schema: in.Output.Schema},
	}
	for _, ex := range in.Examples {
		out.Examples = append(out.Examples, skillv1alpha1.SkillExample{
			Note:   ex.Note,
			Input:  ex.Input,
			Output: ex.Output,
		})
	}
	return out
}

func convertSkillImplementationAlphaToBeta(in skillv1alpha1.SkillImplementation) skillv1beta1.SkillImplementation {
	out := skillv1beta1.SkillImplementation{Type: in.Type}
	if in.Runtime != nil {
		out.Runtime = &skillv1beta1.SkillRuntime{
			Engine:     in.Runtime.Engine,
			Entrypoint: in.Runtime.Entrypoint,
			Image:      in.Runtime.Image,
		}
	}
	if in.PromptTemplate != nil {
		out.PromptTemplate = &skillv1beta1.SkillPromptTemplate{
			Ref:    in.PromptTemplate.Ref,
			Inline: in.PromptTemplate.Inline,
		}
	}
	if in.Requires != nil {
		out.Requires = convertSkillRequiresAlphaToBeta(in.Requires)
	}
	return out
}

func convertSkillImplementationBetaToAlpha(in skillv1beta1.SkillImplementation) skillv1alpha1.SkillImplementation {
	out := skillv1alpha1.SkillImplementation{Type: in.Type}
	if in.Runtime != nil {
		out.Runtime = &skillv1alpha1.SkillRuntime{
			Engine:     in.Runtime.Engine,
			Entrypoint: in.Runtime.Entrypoint,
			Image:      in.Runtime.Image,
		}
	}
	if in.PromptTemplate != nil {
		out.PromptTemplate = &skillv1alpha1.SkillPromptTemplate{
			Ref:    in.PromptTemplate.Ref,
			Inline: in.PromptTemplate.Inline,
		}
	}
	if in.Requires != nil {
		out.Requires = convertSkillRequiresBetaToAlpha(in.Requires)
	}
	return out
}

func convertSkillRequiresAlphaToBeta(in *skillv1alpha1.SkillRequires) *skillv1beta1.SkillRequires {
	out := &skillv1beta1.SkillRequires{}
	for _, m := range in.Models {
		out.Models = append(out.Models, skillv1beta1.SkillModelDep{
			Alias:    m.Alias,
			Ref:      m.Ref,
			Purpose:  m.Purpose,
			Fallback: m.Fallback,
		})
	}
	for _, t := range in.Tools {
		out.Tools = append(out.Tools, skillv1beta1.SkillToolDep{Ref: t.Ref})
	}
	for _, d := range in.DataSources {
		out.DataSources = append(out.DataSources, skillv1beta1.SkillDataSourceDep{Ref: d.Ref})
	}
	for _, s := range in.Skills {
		out.Skills = append(out.Skills, skillv1beta1.SkillSubSkillDep{
			Ref:               s.Ref,
			VersionConstraint: s.VersionConstraint,
		})
	}
	return out
}

func convertSkillRequiresBetaToAlpha(in *skillv1beta1.SkillRequires) *skillv1alpha1.SkillRequires {
	out := &skillv1alpha1.SkillRequires{}
	for _, m := range in.Models {
		out.Models = append(out.Models, skillv1alpha1.SkillModelDep{
			Alias:    m.Alias,
			Ref:      m.Ref,
			Purpose:  m.Purpose,
			Fallback: m.Fallback,
		})
	}
	for _, t := range in.Tools {
		out.Tools = append(out.Tools, skillv1alpha1.SkillToolDep{Ref: t.Ref})
	}
	for _, d := range in.DataSources {
		out.DataSources = append(out.DataSources, skillv1alpha1.SkillDataSourceDep{Ref: d.Ref})
	}
	for _, s := range in.Skills {
		out.Skills = append(out.Skills, skillv1alpha1.SkillSubSkillDep{
			Ref:               s.Ref,
			VersionConstraint: s.VersionConstraint,
		})
	}
	return out
}

func convertSkillEvalAlphaToBeta(in *skillv1alpha1.SkillEvaluation) *skillv1beta1.SkillEvaluation {
	out := &skillv1beta1.SkillEvaluation{
		EvalSet:    in.EvalSet,
		RedTeamSet: in.RedTeamSet,
		Schedule:   in.Schedule,
	}
	if in.Gates != nil {
		out.Gates = make(map[string]map[string]string, len(in.Gates))
		for k, v := range in.Gates {
			inner := make(map[string]string, len(v))
			for mk, mv := range v {
				inner[mk] = mv
			}
			out.Gates[k] = inner
		}
	}
	// Metrics and ContinuousEval are new in beta — left nil.
	return out
}

func convertSkillEvalBetaToAlpha(in *skillv1beta1.SkillEvaluation, lossy []string) (*skillv1alpha1.SkillEvaluation, []string) {
	out := &skillv1alpha1.SkillEvaluation{
		EvalSet:    in.EvalSet,
		RedTeamSet: in.RedTeamSet,
		Schedule:   in.Schedule,
	}
	if in.Gates != nil {
		out.Gates = make(map[string]map[string]string, len(in.Gates))
		for k, v := range in.Gates {
			inner := make(map[string]string, len(v))
			for mk, mv := range v {
				inner[mk] = mv
			}
			out.Gates[k] = inner
		}
	}
	if len(in.Metrics) > 0 {
		lossy = append(lossy, "v1beta1→v1alpha1: spec.evaluation.metrics dropped (no v1alpha1 equivalent)")
	}
	if in.ContinuousEval != nil {
		lossy = append(lossy, "v1beta1→v1alpha1: spec.evaluation.continuousEval dropped (no v1alpha1 equivalent)")
	}
	return out, lossy
}

func convertSkillLifecycleAlphaToBeta(in *skillv1alpha1.SkillLifecycle) *skillv1beta1.SkillLifecycle {
	if in.Deprecation == nil {
		return &skillv1beta1.SkillLifecycle{}
	}
	return &skillv1beta1.SkillLifecycle{
		Deprecation: &skillv1beta1.SkillDeprecation{
			Successor:      in.Deprecation.Successor,
			SunsetAt:       in.Deprecation.SunsetAt,
			MigrationGuide: in.Deprecation.MigrationGuide,
		},
	}
}

func convertSkillLifecycleBetaToAlpha(in *skillv1beta1.SkillLifecycle) *skillv1alpha1.SkillLifecycle {
	if in.Deprecation == nil {
		return &skillv1alpha1.SkillLifecycle{}
	}
	return &skillv1alpha1.SkillLifecycle{
		Deprecation: &skillv1alpha1.SkillDeprecation{
			Successor:      in.Deprecation.Successor,
			SunsetAt:       in.Deprecation.SunsetAt,
			MigrationGuide: in.Deprecation.MigrationGuide,
		},
	}
}

func convertSkillStatusAlphaToBeta(in skillv1alpha1.SkillStatus) skillv1beta1.SkillStatus {
	out := skillv1beta1.SkillStatus{
		Phase:              shared.Phase(in.Phase),
		ObservedGeneration: in.ObservedGeneration,
		ReferencingAgents:  in.ReferencingAgents,
	}
	for _, c := range in.Conditions {
		out.Conditions = append(out.Conditions, c)
	}
	if in.Health != nil {
		out.Health = &skillv1beta1.SkillHealth{
			P95LatencyMs:       in.Health.P95LatencyMs,
			SuccessRate:        in.Health.SuccessRate,
			CostPerCallUsd:     in.Health.CostPerCallUsd,
			Last24hInvocations: in.Health.Last24hInvocations,
		}
	}
	if in.EvalResults != nil {
		out.EvalResults = &skillv1beta1.SkillEvalResults{
			LastRunAt: in.EvalResults.LastRunAt,
			Metrics:   in.EvalResults.Metrics,
			Passed:    in.EvalResults.Passed,
		}
	}
	if in.ResolvedDependencies != nil {
		out.ResolvedDependencies = &skillv1beta1.SkillResolvedDependencies{
			Tools:       in.ResolvedDependencies.Tools,
			DataSources: in.ResolvedDependencies.DataSources,
			Skills:      in.ResolvedDependencies.Skills,
		}
		for _, m := range in.ResolvedDependencies.Models {
			out.ResolvedDependencies.Models = append(out.ResolvedDependencies.Models, skillv1beta1.SkillResolvedModel{
				Alias:       m.Alias,
				ResolvedRef: m.ResolvedRef,
			})
		}
	}
	return out
}

func convertSkillStatusBetaToAlpha(in skillv1beta1.SkillStatus) skillv1alpha1.SkillStatus {
	out := skillv1alpha1.SkillStatus{
		Phase:              shared.Phase(in.Phase),
		ObservedGeneration: in.ObservedGeneration,
		ReferencingAgents:  in.ReferencingAgents,
	}
	for _, c := range in.Conditions {
		out.Conditions = append(out.Conditions, c)
	}
	if in.Health != nil {
		out.Health = &skillv1alpha1.SkillHealth{
			P95LatencyMs:       in.Health.P95LatencyMs,
			SuccessRate:        in.Health.SuccessRate,
			CostPerCallUsd:     in.Health.CostPerCallUsd,
			Last24hInvocations: in.Health.Last24hInvocations,
		}
	}
	if in.EvalResults != nil {
		out.EvalResults = &skillv1alpha1.SkillEvalResults{
			LastRunAt: in.EvalResults.LastRunAt,
			Metrics:   in.EvalResults.Metrics,
			Passed:    in.EvalResults.Passed,
		}
	}
	if in.ResolvedDependencies != nil {
		out.ResolvedDependencies = &skillv1alpha1.SkillResolvedDependencies{
			Tools:       in.ResolvedDependencies.Tools,
			DataSources: in.ResolvedDependencies.DataSources,
			Skills:      in.ResolvedDependencies.Skills,
		}
		for _, m := range in.ResolvedDependencies.Models {
			out.ResolvedDependencies.Models = append(out.ResolvedDependencies.Models, skillv1alpha1.SkillResolvedModel{
				Alias:       m.Alias,
				ResolvedRef: m.ResolvedRef,
			})
		}
	}
	return out
}

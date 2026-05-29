package rollout

import "time"

// Recommendation indicates the action to take based on analysis.
type Recommendation string

const (
	RecommendContinue Recommendation = "Continue"
	RecommendRollback Recommendation = "Rollback"
)

// AnalysisConfig defines thresholds for metric comparison.
type AnalysisConfig struct {
	// ErrorRateThreshold is the maximum acceptable increase in error rate
	// (canary - stable). For example, 0.05 means 5% above stable is tolerated.
	ErrorRateThreshold float64
	// LatencyP95Threshold is the maximum acceptable increase in p95 latency
	// (canary - stable).
	LatencyP95Threshold time.Duration
	// GuardrailTriggerThreshold is the maximum acceptable increase in guardrail
	// trigger count (canary - stable).
	GuardrailTriggerThreshold int
}

// AnalysisMetrics holds observed metrics for a deployment variant.
type AnalysisMetrics struct {
	ErrorRate        float64
	LatencyP95       time.Duration
	GuardrailTriggers int
}

// AnalysisResult contains the outcome of a metric analysis.
type AnalysisResult struct {
	Passed         bool
	FailedMetrics  []string
	Recommendation Recommendation
}

// Analyzer compares canary metrics against stable metrics to decide whether
// to continue a rollout or trigger a rollback.
type Analyzer struct{}

// NewAnalyzer creates a new Analyzer instance.
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze compares canary metrics against stable metrics using the given config.
// If canary's metric degrades relative to stable by more than the threshold,
// the metric is marked as failed and a Rollback is recommended.
func (a *Analyzer) Analyze(canary, stable AnalysisMetrics, config AnalysisConfig) AnalysisResult {
	var failed []string

	// Check error rate degradation.
	if canary.ErrorRate-stable.ErrorRate > config.ErrorRateThreshold {
		failed = append(failed, "ErrorRate")
	}

	// Check p95 latency degradation.
	if canary.LatencyP95-stable.LatencyP95 > config.LatencyP95Threshold {
		failed = append(failed, "LatencyP95")
	}

	// Check guardrail trigger degradation.
	if canary.GuardrailTriggers-stable.GuardrailTriggers > config.GuardrailTriggerThreshold {
		failed = append(failed, "GuardrailTriggers")
	}

	if len(failed) > 0 {
		return AnalysisResult{
			Passed:         false,
			FailedMetrics:  failed,
			Recommendation: RecommendRollback,
		}
	}

	return AnalysisResult{
		Passed:         true,
		FailedMetrics:  nil,
		Recommendation: RecommendContinue,
	}
}

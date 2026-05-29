package rollout

import (
	"testing"
	"time"
)

func TestAnalyzer_AllHealthy(t *testing.T) {
	analyzer := NewAnalyzer()
	config := AnalysisConfig{
		ErrorRateThreshold:        0.05,
		LatencyP95Threshold:       100 * time.Millisecond,
		GuardrailTriggerThreshold: 5,
	}
	canary := AnalysisMetrics{
		ErrorRate:         0.02,
		LatencyP95:        200 * time.Millisecond,
		GuardrailTriggers: 3,
	}
	stable := AnalysisMetrics{
		ErrorRate:         0.01,
		LatencyP95:        180 * time.Millisecond,
		GuardrailTriggers: 2,
	}

	result := analyzer.Analyze(canary, stable, config)

	if !result.Passed {
		t.Errorf("expected Passed=true, got false; failed metrics: %v", result.FailedMetrics)
	}
	if result.Recommendation != RecommendContinue {
		t.Errorf("expected recommendation Continue, got %s", result.Recommendation)
	}
	if len(result.FailedMetrics) != 0 {
		t.Errorf("expected no failed metrics, got %v", result.FailedMetrics)
	}
}

func TestAnalyzer_HighErrorRate(t *testing.T) {
	analyzer := NewAnalyzer()
	config := AnalysisConfig{
		ErrorRateThreshold:        0.05,
		LatencyP95Threshold:       100 * time.Millisecond,
		GuardrailTriggerThreshold: 5,
	}
	canary := AnalysisMetrics{
		ErrorRate:         0.15,
		LatencyP95:        200 * time.Millisecond,
		GuardrailTriggers: 2,
	}
	stable := AnalysisMetrics{
		ErrorRate:         0.02,
		LatencyP95:        180 * time.Millisecond,
		GuardrailTriggers: 2,
	}

	result := analyzer.Analyze(canary, stable, config)

	if result.Passed {
		t.Error("expected Passed=false, got true")
	}
	if result.Recommendation != RecommendRollback {
		t.Errorf("expected recommendation Rollback, got %s", result.Recommendation)
	}
	if len(result.FailedMetrics) != 1 || result.FailedMetrics[0] != "ErrorRate" {
		t.Errorf("expected FailedMetrics=[ErrorRate], got %v", result.FailedMetrics)
	}
}

func TestAnalyzer_HighLatency(t *testing.T) {
	analyzer := NewAnalyzer()
	config := AnalysisConfig{
		ErrorRateThreshold:        0.05,
		LatencyP95Threshold:       100 * time.Millisecond,
		GuardrailTriggerThreshold: 5,
	}
	canary := AnalysisMetrics{
		ErrorRate:         0.01,
		LatencyP95:        500 * time.Millisecond,
		GuardrailTriggers: 2,
	}
	stable := AnalysisMetrics{
		ErrorRate:         0.01,
		LatencyP95:        180 * time.Millisecond,
		GuardrailTriggers: 2,
	}

	result := analyzer.Analyze(canary, stable, config)

	if result.Passed {
		t.Error("expected Passed=false, got true")
	}
	if result.Recommendation != RecommendRollback {
		t.Errorf("expected recommendation Rollback, got %s", result.Recommendation)
	}
	if len(result.FailedMetrics) != 1 || result.FailedMetrics[0] != "LatencyP95" {
		t.Errorf("expected FailedMetrics=[LatencyP95], got %v", result.FailedMetrics)
	}
}

func TestAnalyzer_HighGuardrailTriggers(t *testing.T) {
	analyzer := NewAnalyzer()
	config := AnalysisConfig{
		ErrorRateThreshold:        0.05,
		LatencyP95Threshold:       100 * time.Millisecond,
		GuardrailTriggerThreshold: 5,
	}
	canary := AnalysisMetrics{
		ErrorRate:         0.01,
		LatencyP95:        200 * time.Millisecond,
		GuardrailTriggers: 15,
	}
	stable := AnalysisMetrics{
		ErrorRate:         0.01,
		LatencyP95:        180 * time.Millisecond,
		GuardrailTriggers: 3,
	}

	result := analyzer.Analyze(canary, stable, config)

	if result.Passed {
		t.Error("expected Passed=false, got true")
	}
	if result.Recommendation != RecommendRollback {
		t.Errorf("expected recommendation Rollback, got %s", result.Recommendation)
	}
	if len(result.FailedMetrics) != 1 || result.FailedMetrics[0] != "GuardrailTriggers" {
		t.Errorf("expected FailedMetrics=[GuardrailTriggers], got %v", result.FailedMetrics)
	}
}

func TestAnalyzer_MultipleFailures(t *testing.T) {
	analyzer := NewAnalyzer()
	config := AnalysisConfig{
		ErrorRateThreshold:        0.05,
		LatencyP95Threshold:       100 * time.Millisecond,
		GuardrailTriggerThreshold: 5,
	}
	canary := AnalysisMetrics{
		ErrorRate:         0.20,
		LatencyP95:        500 * time.Millisecond,
		GuardrailTriggers: 20,
	}
	stable := AnalysisMetrics{
		ErrorRate:         0.01,
		LatencyP95:        180 * time.Millisecond,
		GuardrailTriggers: 3,
	}

	result := analyzer.Analyze(canary, stable, config)

	if result.Passed {
		t.Error("expected Passed=false, got true")
	}
	if result.Recommendation != RecommendRollback {
		t.Errorf("expected recommendation Rollback, got %s", result.Recommendation)
	}
	if len(result.FailedMetrics) != 3 {
		t.Errorf("expected 3 failed metrics, got %d: %v", len(result.FailedMetrics), result.FailedMetrics)
	}
	// Verify all three metrics are reported.
	expected := map[string]bool{"ErrorRate": true, "LatencyP95": true, "GuardrailTriggers": true}
	for _, m := range result.FailedMetrics {
		if !expected[m] {
			t.Errorf("unexpected failed metric: %s", m)
		}
		delete(expected, m)
	}
	if len(expected) > 0 {
		t.Errorf("missing failed metrics: %v", expected)
	}
}

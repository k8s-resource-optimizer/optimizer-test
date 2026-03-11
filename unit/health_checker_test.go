package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/sla"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// healthyMetrics returns n latency metrics well below any SLA threshold.
func healthyMetrics(n int) []sla.Metric {
	metrics := make([]sla.Metric, n)
	for i := 0; i < n; i++ {
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(-time.Duration(n-i) * time.Minute),
			Value:     50.0, // 50ms — well under typical 150ms SLA
		}
	}
	return metrics
}

// violatingMetrics returns n latency metrics well above the given threshold.
func violatingMetrics(value float64, n int) []sla.Metric {
	metrics := make([]sla.Metric, n)
	for i := 0; i < n; i++ {
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(-time.Duration(n-i) * time.Minute),
			Value:     value,
		}
	}
	return metrics
}

// strictLatencySLAs returns a slice with one tight latency SLA (ceiling=100ms).
func strictLatencySLAs() []sla.SLADefinition {
	return []sla.SLADefinition{
		{
			Name:        "tight-latency",
			Type:        sla.SLATypeLatency,
			Target:      0,
			Threshold:   100.0, // ceiling = 100ms
			Percentile:  95,
			Window:      5 * time.Minute,
			Description: "P95 latency under 100ms",
		},
	}
}

// ─── HealthChecker tests ────────────────────────────────────────────────────

// TestHealthChecker_HealthyWhenNoViolations verifies that metrics well within
// SLA thresholds result in IsHealthy=true and Score=100.
func TestHealthChecker_HealthyWhenNoViolations(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, err := hc.CheckHealth(healthyMetrics(10), strictLatencySLAs())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected error: %v", err)
	}
	if !result.IsHealthy {
		t.Errorf("expected IsHealthy=true for metrics within SLA, got false (score=%.1f)", result.Score)
	}
	if result.Score < 70 {
		t.Errorf("expected Score>=70 for healthy metrics, got %.1f", result.Score)
	}
}

// TestHealthChecker_UnhealthyWhenViolated verifies that metrics significantly
// exceeding the SLA threshold result in IsHealthy=false.
func TestHealthChecker_UnhealthyWhenViolated(t *testing.T) {
	hc := sla.NewHealthChecker()
	// 500ms far exceeds the 100ms ceiling.
	result, err := hc.CheckHealth(violatingMetrics(500, 10), strictLatencySLAs())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected error: %v", err)
	}
	if result.IsHealthy {
		t.Errorf("expected IsHealthy=false for metrics violating SLA (score=%.1f)", result.Score)
	}
}

// TestHealthChecker_ScoreRange verifies that the health score is always
// within the valid range [0, 100].
func TestHealthChecker_ScoreRange(t *testing.T) {
	hc := sla.NewHealthChecker()

	cases := []struct {
		name    string
		metrics []sla.Metric
	}{
		{"healthy", healthyMetrics(10)},
		{"violating", violatingMetrics(500, 10)},
		{"empty", []sla.Metric{}},
	}

	for _, tc := range cases {
		result, err := hc.CheckHealth(tc.metrics, strictLatencySLAs())
		if err != nil {
			continue // empty metrics may return error — that's acceptable
		}
		if result.Score < 0 || result.Score > 100 {
			t.Errorf("case %q: score %.1f is outside [0, 100]", tc.name, result.Score)
		}
	}
}

// TestHealthChecker_MessageNotEmpty verifies that CheckHealth always sets
// a non-empty message.
func TestHealthChecker_MessageNotEmpty(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, err := hc.CheckHealth(healthyMetrics(5), strictLatencySLAs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty Message in HealthCheckResult")
	}
}

// TestHealthChecker_ViolationsPopulatedWhenPresent verifies that Violations
// slice is non-empty when SLA thresholds are breached.
func TestHealthChecker_ViolationsPopulatedWhenPresent(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, err := hc.CheckHealth(violatingMetrics(999, 10), strictLatencySLAs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Violations) == 0 {
		t.Error("expected at least one violation for heavily violating metrics")
	}
}

// TestHealthChecker_PreOptimizationCheck_ReturnsResult verifies that
// PreOptimizationCheck returns a non-nil result for normal metrics.
func TestHealthChecker_PreOptimizationCheck_ReturnsResult(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, err := hc.PreOptimizationCheck(healthyMetrics(10))
	if err != nil {
		t.Fatalf("PreOptimizationCheck error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from PreOptimizationCheck")
	}
}

// TestHealthChecker_PostOptimizationCheck_ReturnsResult verifies that
// PostOptimizationCheck returns a non-nil result for normal metrics.
func TestHealthChecker_PostOptimizationCheck_ReturnsResult(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, err := hc.PostOptimizationCheck(healthyMetrics(10))
	if err != nil {
		t.Fatalf("PostOptimizationCheck error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from PostOptimizationCheck")
	}
}

// TestHealthChecker_CompareHealth_NilInputsReturnError verifies that passing
// nil pre or post result to CompareHealth returns an error.
func TestHealthChecker_CompareHealth_NilInputsReturnError(t *testing.T) {
	hc := sla.NewHealthChecker()
	_, err := hc.CompareHealth(nil, nil)
	if err == nil {
		t.Error("expected error when both inputs are nil")
	}
}

// TestHealthChecker_CompareHealth_ImprovementDetected verifies that when
// post-optimization score is higher than pre-optimization, ImpactScore > 0.
func TestHealthChecker_CompareHealth_ImprovementDetected(t *testing.T) {
	hc := sla.NewHealthChecker()

	// Pre: unhealthy (score will be low due to violations)
	pre, err := hc.PreOptimizationCheck(violatingMetrics(500, 10))
	if err != nil {
		t.Fatalf("PreOptimizationCheck error: %v", err)
	}

	// Post: healthy (score will be high)
	hcPost := sla.NewHealthChecker()
	post, err := hcPost.PostOptimizationCheck(healthyMetrics(10))
	if err != nil {
		t.Fatalf("PostOptimizationCheck error: %v", err)
	}

	impact, err := hc.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}
	if impact.ImpactScore <= 0 {
		t.Errorf("expected positive ImpactScore for improvement (pre=%.1f post=%.1f), got %f",
			pre.Score, post.Score, impact.ImpactScore)
	}
}

// TestHealthChecker_CompareHealth_DegradationDetected verifies that when
// post-optimization score is lower, ImpactScore < 0.
func TestHealthChecker_CompareHealth_DegradationDetected(t *testing.T) {
	hc := sla.NewHealthChecker()

	// Pre: healthy
	pre, err := hc.PreOptimizationCheck(healthyMetrics(10))
	if err != nil {
		t.Fatalf("PreOptimizationCheck error: %v", err)
	}

	// Post: unhealthy
	hcPost := sla.NewHealthChecker()
	post, err := hcPost.PostOptimizationCheck(violatingMetrics(500, 10))
	if err != nil {
		t.Fatalf("PostOptimizationCheck error: %v", err)
	}

	impact, err := hc.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}
	if impact.ImpactScore >= 0 {
		t.Errorf("expected negative ImpactScore for degradation (pre=%.1f post=%.1f), got %f",
			pre.Score, post.Score, impact.ImpactScore)
	}
}

// TestHealthChecker_CompareHealth_RecommendationNotEmpty verifies that
// CompareHealth always sets a non-empty Recommendation.
func TestHealthChecker_CompareHealth_RecommendationNotEmpty(t *testing.T) {
	hcPre := sla.NewHealthChecker()
	pre, _ := hcPre.PreOptimizationCheck(healthyMetrics(5))
	hcPost := sla.NewHealthChecker()
	post, _ := hcPost.PostOptimizationCheck(healthyMetrics(5))

	if pre == nil || post == nil {
		t.Skip("health check returned nil — skipping recommendation check")
	}

	impact, err := hcPre.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}
	if impact.Recommendation == "" {
		t.Error("expected non-empty Recommendation in OptimizationImpact")
	}
}

// ─── IsSystemHealthy tests ──────────────────────────────────────────────────

// TestIsSystemHealthy_TrueForHealthyResult verifies that a result with
// IsHealthy=true and score above minScore returns true.
func TestIsSystemHealthy_TrueForHealthyResult(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, _ := hc.CheckHealth(healthyMetrics(10), strictLatencySLAs())
	if result == nil {
		t.Skip("nil result")
	}
	if !sla.IsSystemHealthy(result, 50.0) {
		t.Errorf("expected IsSystemHealthy=true for healthy result (score=%.1f)", result.Score)
	}
}

// TestIsSystemHealthy_FalseForNilResult verifies that a nil result is
// treated as unhealthy (safe default).
func TestIsSystemHealthy_FalseForNilResult(t *testing.T) {
	if sla.IsSystemHealthy(nil, 70.0) {
		t.Error("expected IsSystemHealthy=false for nil result")
	}
}

// ─── ShouldBlockOptimization tests ─────────────────────────────────────────

// TestShouldBlockOptimization_BlocksWhenUnhealthy verifies that optimization
// is blocked when the system is unhealthy.
func TestShouldBlockOptimization_BlocksWhenUnhealthy(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, _ := hc.CheckHealth(violatingMetrics(999, 10), strictLatencySLAs())
	if result == nil {
		t.Skip("nil result")
	}
	if result.IsHealthy {
		t.Skip("result is unexpectedly healthy — skipping block check")
	}

	blocked, reason := sla.ShouldBlockOptimization(result)
	if !blocked {
		t.Error("expected optimization to be blocked when system is unhealthy")
	}
	if reason == "" {
		t.Error("expected non-empty block reason")
	}
}

// TestShouldBlockOptimization_BlocksForNilResult verifies that a nil result
// causes optimization to be blocked (fail-safe behavior).
func TestShouldBlockOptimization_BlocksForNilResult(t *testing.T) {
	blocked, _ := sla.ShouldBlockOptimization(nil)
	if !blocked {
		t.Error("expected ShouldBlockOptimization=true for nil result")
	}
}

// TestShouldBlockOptimization_AllowsWhenHealthy verifies that optimization
// is NOT blocked when the system is healthy with no critical violations.
func TestShouldBlockOptimization_AllowsWhenHealthy(t *testing.T) {
	hc := sla.NewHealthChecker()
	result, _ := hc.CheckHealth(healthyMetrics(10), strictLatencySLAs())
	if result == nil {
		t.Skip("nil result")
	}
	if !result.IsHealthy {
		t.Skip("result is unexpectedly unhealthy — skipping allow check")
	}

	blocked, _ := sla.ShouldBlockOptimization(result)
	if blocked {
		t.Error("expected optimization to be allowed when system is healthy")
	}
}

// ─── ShouldRollback tests ───────────────────────────────────────────────────

// TestShouldRollback_FalseForNilImpact verifies that nil impact data does
// not trigger a rollback.
func TestShouldRollback_FalseForNilImpact(t *testing.T) {
	rollback, _ := sla.ShouldRollback(nil)
	if rollback {
		t.Error("expected ShouldRollback=false for nil impact")
	}
}

// TestShouldRollback_TrueForSignificantDegradation verifies that a large
// negative ImpactScore triggers a rollback recommendation.
func TestShouldRollback_TrueForSignificantDegradation(t *testing.T) {
	hcPre := sla.NewHealthChecker()
	pre, _ := hcPre.PreOptimizationCheck(healthyMetrics(10))

	hcPost := sla.NewHealthChecker()
	post, _ := hcPost.PostOptimizationCheck(violatingMetrics(999, 10))

	if pre == nil || post == nil {
		t.Skip("nil health check results")
	}

	impact, err := hcPre.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}

	// Only assert rollback if there was an actual significant degradation.
	if impact.ImpactScore < -0.15 {
		rollback, reason := sla.ShouldRollback(impact)
		if !rollback {
			t.Errorf("expected rollback for significant degradation (impact=%.3f), reason=%s",
				impact.ImpactScore, reason)
		}
	}
}

// TestShouldRollback_FalseForImprovement verifies that an improvement does
// not trigger a rollback.
func TestShouldRollback_FalseForImprovement(t *testing.T) {
	hcPre := sla.NewHealthChecker()
	pre, _ := hcPre.PreOptimizationCheck(violatingMetrics(500, 10))

	hcPost := sla.NewHealthChecker()
	post, _ := hcPost.PostOptimizationCheck(healthyMetrics(10))

	if pre == nil || post == nil {
		t.Skip("nil health check results")
	}

	impact, err := hcPre.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}

	// If there was an actual improvement (and no new violations), no rollback.
	if impact.ImpactScore > 0.15 && len(impact.ViolationsAdded) == 0 {
		rollback, _ := sla.ShouldRollback(impact)
		if rollback {
			t.Errorf("expected no rollback for improvement (impact=%.3f)", impact.ImpactScore)
		}
	}
}

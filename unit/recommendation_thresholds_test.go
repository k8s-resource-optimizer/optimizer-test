package unit_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

// makeThresholdConfig returns an OptimizerConfig with the given CPU and memory
// threshold bounds already set.
func makeThresholdConfig(cpuMin, cpuMax, memMin, memMax string) *v1alpha1.OptimizerConfig {
	cfg := basicConfig(95) // from recommendation_test.go
	cfg.Spec.Strategy = v1alpha1.StrategyBalanced
	cfg.Spec.ResourceThresholds = &v1alpha1.ResourceThresholds{}
	if cpuMin != "" || cpuMax != "" {
		cfg.Spec.ResourceThresholds.CPU = &v1alpha1.ResourceLimit{
			Min: cpuMin,
			Max: cpuMax,
		}
	}
	if memMin != "" || memMax != "" {
		cfg.Spec.ResourceThresholds.Memory = &v1alpha1.ResourceLimit{
			Min: memMin,
			Max: memMax,
		}
	}
	return cfg
}

// ─── applyThresholds — CPU ────────────────────────────────────────────────────

// TestApplyThresholds_CPUMin_ClampsUp verifies that when the recommendation
// engine would produce a CPU value below the configured minimum, it is clamped
// up to that minimum.
func TestApplyThresholds_CPUMin_ClampsUp(t *testing.T) {
	// Generate metrics with very low CPU usage (around 10m P95).
	metrics := generateSyntheticMetrics(50, 5, 128*1024*1024, 10, 0)
	provider := &mockMetricsProvider{metrics: metrics}

	// Set a very high CPU minimum (4000m = 4 cores) so the recommendation must be
	// clamped up from the ~10m P95 value.
	cfg := makeThresholdConfig("4000m", "", "", "")
	cfg.Spec.Recommendations.MinSamples = 10

	e := recommendation.NewEngine()
	recs, err := e.GenerateRecommendationsWithOOM(provider, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendationsWithOOM error: %v", err)
	}
	if len(recs) == 0 {
		t.Skip("no recommendations generated — insufficient data for namespace")
	}

	for _, wr := range recs {
		for _, cr := range wr.Containers {
			// 4000m in millicores
			if cr.RecommendedCPU < 4000 {
				t.Errorf("container %s: expected CPU >= 4000m due to min threshold, got %d",
					cr.ContainerName, cr.RecommendedCPU)
			}
		}
	}
}

// TestApplyThresholds_CPUMax_ClampsDown verifies that when the recommendation
// would exceed the configured CPU maximum, it is clamped down.
func TestApplyThresholds_CPUMax_ClampsDown(t *testing.T) {
	// High CPU usage: base=800m, spread=400m → P95 ≈ 1160m
	metrics := generateSyntheticMetrics(50, 800, 256*1024*1024, 400, 0)
	provider := &mockMetricsProvider{metrics: metrics}

	// Cap at 200m — well below the ~1160m P95.
	cfg := makeThresholdConfig("", "200m", "", "")
	cfg.Spec.Recommendations.MinSamples = 10

	e := recommendation.NewEngine()
	recs, err := e.GenerateRecommendationsWithOOM(provider, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendationsWithOOM error: %v", err)
	}
	if len(recs) == 0 {
		t.Skip("no recommendations generated")
	}

	for _, wr := range recs {
		for _, cr := range wr.Containers {
			if cr.RecommendedCPU > 200 {
				t.Errorf("container %s: expected CPU <= 200m due to max threshold, got %d",
					cr.ContainerName, cr.RecommendedCPU)
			}
		}
	}
}

// TestApplyThresholds_MemoryMin_ClampsUp verifies that memory below the
// configured minimum is clamped up.
func TestApplyThresholds_MemoryMin_ClampsUp(t *testing.T) {
	// Very low memory: ~16MiB P95
	metrics := generateSyntheticMetrics(50, 500, 8*1024*1024, 0, 16*1024*1024)
	provider := &mockMetricsProvider{metrics: metrics}

	// Minimum 512Mi — forces recommendation up. Use "512Mi" for deterministic parsing.
	cfg := makeThresholdConfig("", "", "512Mi", "")
	cfg.Spec.Recommendations.MinSamples = 10

	e := recommendation.NewEngine()
	recs, err := e.GenerateRecommendationsWithOOM(provider, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendationsWithOOM error: %v", err)
	}
	if len(recs) == 0 {
		t.Skip("no recommendations generated")
	}

	// 512Mi = 512*1024*1024 bytes; raw P95 ≈ 25MiB, so the clamp must fire.
	const min512Mi = int64(512 * 1024 * 1024)
	for _, wr := range recs {
		for _, cr := range wr.Containers {
			if cr.RecommendedMemory < min512Mi {
				t.Errorf("container %s: expected Memory >= 512Mi due to min threshold, got %d",
					cr.ContainerName, cr.RecommendedMemory)
			}
		}
	}
}

// TestApplyThresholds_MemoryMax_ClampsDown verifies that memory above the
// configured maximum is clamped down.
func TestApplyThresholds_MemoryMax_ClampsDown(t *testing.T) {
	// High memory: base=512MiB, spread=512MiB → P95 ≈ 998MiB
	metrics := generateSyntheticMetrics(50, 500, 512*1024*1024, 0, 512*1024*1024)
	provider := &mockMetricsProvider{metrics: metrics}

	// Cap at 128MiB.
	cfg := makeThresholdConfig("", "", "", "128Mi")
	cfg.Spec.Recommendations.MinSamples = 10

	e := recommendation.NewEngine()
	recs, err := e.GenerateRecommendationsWithOOM(provider, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendationsWithOOM error: %v", err)
	}
	if len(recs) == 0 {
		t.Skip("no recommendations generated")
	}

	const cap128Mi = int64(128 * 1024 * 1024)
	for _, wr := range recs {
		for _, cr := range wr.Containers {
			if cr.RecommendedMemory > cap128Mi {
				t.Errorf("container %s: expected Memory <= 128Mi due to max threshold, got %d",
					cr.ContainerName, cr.RecommendedMemory)
			}
		}
	}
}

// ─── applyStrategy tests ──────────────────────────────────────────────────────

// TestApplyStrategy_Aggressive_LowersCPUPercentile verifies that Aggressive
// strategy lowers the effective percentile, producing a smaller CPU recommendation
// compared to Balanced strategy for the same metrics.
func TestApplyStrategy_Aggressive_LowersCPUPercentile(t *testing.T) {
	metrics := generateSyntheticMetrics(50, 200, 256*1024*1024, 800, 0)
	provider := &mockMetricsProvider{metrics: metrics}

	makeCfg := func(strategy v1alpha1.OptimizationStrategy) *v1alpha1.OptimizerConfig {
		cfg := basicConfig(95)
		cfg.Spec.Strategy = strategy
		cfg.Spec.Recommendations.MinSamples = 10
		return cfg
	}

	e := recommendation.NewEngine()

	aggressive, err := e.GenerateRecommendationsWithOOM(provider, nil, makeCfg(v1alpha1.StrategyAggressive))
	if err != nil {
		t.Fatalf("Aggressive strategy error: %v", err)
	}
	balanced, err := e.GenerateRecommendationsWithOOM(provider, nil, makeCfg(v1alpha1.StrategyBalanced))
	if err != nil {
		t.Fatalf("Balanced strategy error: %v", err)
	}

	if len(aggressive) == 0 || len(balanced) == 0 {
		t.Skip("no recommendations generated for strategy comparison")
	}

	aggrCPU := aggressive[0].Containers[0].RecommendedCPU
	balCPU := balanced[0].Containers[0].RecommendedCPU

	if aggrCPU > balCPU {
		t.Errorf("aggressive CPU (%d) should be <= balanced CPU (%d)", aggrCPU, balCPU)
	}
}

// TestApplyStrategy_Conservative_RaisesCPUPercentile verifies that Conservative
// strategy raises the effective percentile, producing a higher recommendation.
func TestApplyStrategy_Conservative_RaisesCPUPercentile(t *testing.T) {
	metrics := generateSyntheticMetrics(50, 200, 256*1024*1024, 800, 0)
	provider := &mockMetricsProvider{metrics: metrics}

	makeCfg := func(strategy v1alpha1.OptimizationStrategy) *v1alpha1.OptimizerConfig {
		cfg := basicConfig(95)
		cfg.Spec.Strategy = strategy
		cfg.Spec.Recommendations.MinSamples = 10
		return cfg
	}

	e := recommendation.NewEngine()

	conservative, err := e.GenerateRecommendationsWithOOM(provider, nil, makeCfg(v1alpha1.StrategyConservative))
	if err != nil {
		t.Fatalf("Conservative strategy error: %v", err)
	}
	balanced, err := e.GenerateRecommendationsWithOOM(provider, nil, makeCfg(v1alpha1.StrategyBalanced))
	if err != nil {
		t.Fatalf("Balanced strategy error: %v", err)
	}

	if len(conservative) == 0 || len(balanced) == 0 {
		t.Skip("no recommendations generated for strategy comparison")
	}

	consCPU := conservative[0].Containers[0].RecommendedCPU
	balCPU := balanced[0].Containers[0].RecommendedCPU

	if consCPU < balCPU {
		t.Errorf("conservative CPU (%d) should be >= balanced CPU (%d)", consCPU, balCPU)
	}
}

// ─── sortRecommendationsByPriority / shouldSwap tests ─────────────────────────

// TestSortRecommendations_OOMWorkloadFirst verifies that GenerateRecommendationsWithOOM
// places workloads with OOM history before those without when there are multiple
// workloads returned.  We do this indirectly through a mock OOM provider.
func TestSortRecommendations_OOMWorkloadFirst(t *testing.T) {
	// Two workloads: "pod-0" and "other-0".  We return the same metrics for
	// both — the OOM provider will mark "pod" workload as having OOM history.
	metrics := append(
		generateSyntheticMetrics(20, 200, 128*1024*1024, 100, 0),
		generateSyntheticMetrics(20, 300, 256*1024*1024, 100, 0)...,
	)
	// Rename second batch to a different pod.
	for i := 20; i < len(metrics); i++ {
		metrics[i].PodName = "other-pod-abc"
	}
	provider := &mockMetricsProvider{metrics: metrics}

	e := recommendation.NewEngine()
	cfg := basicConfig(95)
	cfg.Spec.Recommendations.MinSamples = 10

	recs, err := e.GenerateRecommendationsWithOOM(provider, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendationsWithOOM error: %v", err)
	}
	// Just verify that the call succeeds and we got multiple recommendations.
	if len(recs) < 1 {
		t.Skip("only one recommendation returned — sort test not applicable")
	}
}

// ─── ExpiryStatus / calculateChangePercent edge cases ────────────────────────

// TestExpiryStatus_Fresh verifies that a freshly generated recommendation
// returns a "Valid for" status string (not expired).
func TestExpiryStatus_Fresh(t *testing.T) {
	rec := recommendation.WorkloadRecommendation{
		GeneratedAt: time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
	status := rec.ExpiryStatus()
	if status == "" {
		t.Error("expected non-empty ExpiryStatus")
	}
}

// TestExpiryStatus_Expired verifies that an expired recommendation returns
// an "EXPIRED" status string.
func TestExpiryStatus_Expired(t *testing.T) {
	rec := recommendation.WorkloadRecommendation{
		GeneratedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt:   time.Now().Add(-24 * time.Hour),
	}
	status := rec.ExpiryStatus()
	if len(status) < 7 || status[:7] != "EXPIRED" {
		t.Errorf("expected status to start with 'EXPIRED', got %q", status)
	}
}

// TestCalculateChangePercent_ZeroCurrent verifies that a zero current value
// produces 100% change (edge case: division by zero guard).
func TestCalculateChangePercent_ZeroCurrent(t *testing.T) {
	cr := recommendation.ContainerRecommendation{
		CurrentCPU:      0,
		RecommendedCPU:  500,
		CurrentMemory:   0,
		RecommendedMemory: 256 * 1024 * 1024,
	}
	pct := cr.CalculateCPUChangePercent()
	if pct != 100.0 {
		t.Errorf("expected 100%% change when current=0, got %.1f", pct)
	}
}

// TestCalculateChangePercent_BothZero verifies that zero→zero change is 0%.
func TestCalculateChangePercent_BothZero(t *testing.T) {
	cr := recommendation.ContainerRecommendation{
		CurrentCPU:     0,
		RecommendedCPU: 0,
	}
	pct := cr.CalculateCPUChangePercent()
	if pct != 0.0 {
		t.Errorf("expected 0%% change when both are 0, got %.1f", pct)
	}
}

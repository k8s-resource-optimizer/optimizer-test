package integration_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/pareto"
)

func makeWorkloadMetrics(ns, name string) *pareto.WorkloadMetrics {
	return &pareto.WorkloadMetrics{
		Namespace:    ns,
		WorkloadName: name,
		CurrentCPU:   int64(1000),
		CurrentMemory: int64(512 * 1024 * 1024),
		AvgCPU:       int64(300),
		AvgMemory:    int64(256 * 1024 * 1024),
		PeakCPU:      int64(900),
		PeakMemory:   int64(480 * 1024 * 1024),
		P95CPU:       int64(700),
		P95Memory:    int64(400 * 1024 * 1024),
		P99CPU:       int64(850),
		P99Memory:    int64(450 * 1024 * 1024),
		Confidence:  85.0,
		SampleCount: 200,
	}
}

// TestParetoExtendedPipeline_RecommendationHelper verifies the full
// RecommendationHelper pipeline: GenerateRecommendation → result.
func TestParetoExtendedPipeline_RecommendationHelper(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	metrics := makeWorkloadMetrics("default", "my-app")

	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil recommendation result")
	}
}

// TestParetoExtendedPipeline_RecommendationHelperWithOptimizer verifies
// NewRecommendationHelperWithOptimizer constructs correctly.
func TestParetoExtendedPipeline_RecommendationHelperWithOptimizer(t *testing.T) {
	opt := pareto.NewOptimizer()
	helper := pareto.NewRecommendationHelperWithOptimizer(opt)

	metrics := makeWorkloadMetrics("production", "worker")
	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation error: %v", err)
	}
	_ = result
}

// TestParetoExtendedPipeline_GetRecommendationForProfile verifies profile-based selection.
func TestParetoExtendedPipeline_GetRecommendationForProfile(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	metrics := makeWorkloadMetrics("default", "frontend")

	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation error: %v", err)
	}

	for _, profile := range []string{"production", "development", "staging", "cost-optimized"} {
		sol := helper.GetRecommendationForProfile(result, profile)
		if sol == nil {
			t.Errorf("expected non-nil solution for profile %q", profile)
		}
	}
}

// TestParetoExtendedPipeline_CompareStrategies verifies CompareStrategies output.
func TestParetoExtendedPipeline_CompareStrategies(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	metrics := makeWorkloadMetrics("default", "api")

	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation error: %v", err)
	}

	comparisons := helper.CompareStrategies(result)
	_ = comparisons
}

// TestParetoExtendedPipeline_AnalyzeTradeOffs verifies AnalyzeTradeOffs.
func TestParetoExtendedPipeline_AnalyzeTradeOffs(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	metrics := makeWorkloadMetrics("default", "cache")

	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation error: %v", err)
	}

	tradeoffs := helper.AnalyzeTradeOffs(result)
	_ = tradeoffs
}

// TestParetoExtendedPipeline_SetObjectiveWeights verifies custom weights.
func TestParetoExtendedPipeline_SetObjectiveWeights(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	helper.SetObjectiveWeights(0.5, 0.3, 0.2, 0.0, 0.0)

	metrics := makeWorkloadMetrics("default", "weighted-app")
	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation with custom weights error: %v", err)
	}
	_ = result
}

// TestParetoExtendedPipeline_SetCostParameters verifies cost parameter injection.
func TestParetoExtendedPipeline_SetCostParameters(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	helper.SetCostParameters(0.048, 0.006) // cpu $/core-hr, mem $/GiB-hr

	metrics := makeWorkloadMetrics("default", "costed-app")
	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation with cost params error: %v", err)
	}
	_ = result
}

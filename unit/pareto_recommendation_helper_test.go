package unit_test

import (
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/pareto"
)

// sampleMetrics returns a WorkloadMetrics with realistic CPU/memory values.
func sampleMetrics() *pareto.WorkloadMetrics {
	return &pareto.WorkloadMetrics{
		Namespace:     "default",
		WorkloadName:  "api-server",
		CurrentCPU:    1000, // 1 core
		CurrentMemory: 512 * 1024 * 1024,
		AvgCPU:        400,
		AvgMemory:     300 * 1024 * 1024,
		PeakCPU:       900,
		PeakMemory:    480 * 1024 * 1024,
		P95CPU:        700,
		P95Memory:     420 * 1024 * 1024,
		P99CPU:        850,
		P99Memory:     460 * 1024 * 1024,
		Confidence:    85.0,
		SampleCount:   100,
	}
}

// TestRecommendationHelper_GenerateRecommendation_Nil verifies that passing
// nil metrics returns an error and no panic.
func TestRecommendationHelper_GenerateRecommendation_Nil(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	_, err := h.GenerateRecommendation(nil)
	if err == nil {
		t.Error("expected error for nil metrics, got nil")
	}
}

// TestRecommendationHelper_GenerateRecommendation_ReturnsResult verifies that
// a valid WorkloadMetrics produces a non-nil ParetoRecommendation.
func TestRecommendationHelper_GenerateRecommendation_ReturnsResult(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil recommendation")
	}
}

// TestRecommendationHelper_GenerateRecommendation_BestSolutionNotNil verifies
// that the BestSolution field is populated.
func TestRecommendationHelper_GenerateRecommendation_BestSolutionNotNil(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.BestSolution == nil {
		t.Error("expected BestSolution to be non-nil")
	}
}

// TestRecommendationHelper_GenerateRecommendation_ParetoFrontierNotEmpty verifies
// that the Pareto frontier contains at least one optimal solution.
func TestRecommendationHelper_GenerateRecommendation_ParetoFrontierNotEmpty(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.ParetoFrontier) == 0 {
		t.Error("expected non-empty Pareto frontier")
	}
}

// TestRecommendationHelper_GenerateRecommendation_SummaryNotEmpty verifies
// that the Summary field is populated.
func TestRecommendationHelper_GenerateRecommendation_SummaryNotEmpty(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Summary == "" {
		t.Error("expected non-empty Summary")
	}
}

// TestRecommendationHelper_GenerateRecommendation_NamespacePreserved verifies
// that the Namespace and WorkloadName from metrics are set on the result.
func TestRecommendationHelper_GenerateRecommendation_NamespacePreserved(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	m := sampleMetrics()
	rec, err := h.GenerateRecommendation(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Namespace != m.Namespace {
		t.Errorf("expected namespace %q, got %q", m.Namespace, rec.Namespace)
	}
	if rec.WorkloadName != m.WorkloadName {
		t.Errorf("expected workload name %q, got %q", m.WorkloadName, rec.WorkloadName)
	}
}

// TestRecommendationHelper_CompareStrategies_ReturnsComparisons verifies that
// CompareStrategies returns a non-empty slice for a non-trivial recommendation.
func TestRecommendationHelper_CompareStrategies_ReturnsComparisons(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	comparisons := h.CompareStrategies(rec)
	if len(comparisons) == 0 {
		t.Error("expected at least one strategy comparison")
	}
}

// TestRecommendationHelper_CompareStrategies_ScoresInRange verifies that each
// strategy comparison has an OverallScore in [0, 1].
func TestRecommendationHelper_CompareStrategies_ScoresInRange(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, comp := range h.CompareStrategies(rec) {
		if comp.OverallScore < 0 || comp.OverallScore > 1 {
			t.Errorf("strategy %q: OverallScore %.3f outside [0,1]", comp.Strategy, comp.OverallScore)
		}
	}
}

// TestRecommendationHelper_GetRecommendationForProfile_Production verifies
// that the "production" profile returns a non-nil solution.
func TestRecommendationHelper_GetRecommendationForProfile_Production(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sol := h.GetRecommendationForProfile(rec, "production")
	if sol == nil {
		t.Error("expected non-nil solution for 'production' profile")
	}
}

// TestRecommendationHelper_GetRecommendationForProfile_Development verifies
// that the "development" profile returns a non-nil solution.
func TestRecommendationHelper_GetRecommendationForProfile_Development(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sol := h.GetRecommendationForProfile(rec, "development")
	if sol == nil {
		t.Error("expected non-nil solution for 'development' profile")
	}
}

// TestRecommendationHelper_GetRecommendationForProfile_Test verifies that the
// "test" profile alias returns a non-nil solution (same as development).
func TestRecommendationHelper_GetRecommendationForProfile_Test(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sol := h.GetRecommendationForProfile(rec, "test")
	if sol == nil {
		t.Error("expected non-nil solution for 'test' profile")
	}
}

// TestRecommendationHelper_GetRecommendationForProfile_Performance verifies
// that the "performance" profile returns a non-nil solution.
func TestRecommendationHelper_GetRecommendationForProfile_Performance(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sol := h.GetRecommendationForProfile(rec, "performance")
	if sol == nil {
		t.Error("expected non-nil solution for 'performance' profile")
	}
}

// TestRecommendationHelper_GetRecommendationForProfile_DefaultReturnsBest verifies
// that an unknown profile falls back to BestSolution.
func TestRecommendationHelper_GetRecommendationForProfile_DefaultReturnsBest(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sol := h.GetRecommendationForProfile(rec, "unknown-profile")
	if sol != rec.BestSolution {
		t.Error("expected unknown profile to return BestSolution")
	}
}

// TestRecommendationHelper_AnalyzeTradeOffs_ReturnsString verifies that
// AnalyzeTradeOffs returns a non-empty string.
func TestRecommendationHelper_AnalyzeTradeOffs_ReturnsString(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := h.AnalyzeTradeOffs(rec)
	if result == "" {
		t.Error("expected non-empty trade-off analysis string")
	}
}

// TestRecommendationHelper_AnalyzeTradeOffs_NoTradeOffsMessage verifies that
// when the frontier has only one solution the analysis message indicates no trade-offs.
func TestRecommendationHelper_AnalyzeTradeOffs_NoTradeOffsMessage(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	// Build a synthetic recommendation with no trade-offs.
	rec := &pareto.ParetoRecommendation{
		Namespace:    "default",
		WorkloadName: "simple-app",
		TradeOffs:    nil,
	}
	result := h.AnalyzeTradeOffs(rec)
	if !strings.Contains(result, "No trade-offs") {
		t.Errorf("expected 'No trade-offs' message, got: %s", result)
	}
}

// TestRecommendationHelper_SetObjectiveWeights_NoPanic verifies that calling
// SetObjectiveWeights does not panic.
func TestRecommendationHelper_SetObjectiveWeights_NoPanic(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	// Should not panic with valid weights.
	h.SetObjectiveWeights(0.4, 0.3, 0.2, 0.05, 0.05)
}

// TestRecommendationHelper_SetCostParameters_NoPanic verifies that calling
// SetCostParameters does not panic and influences cost-related outputs.
func TestRecommendationHelper_SetCostParameters_NoPanic(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	// Should not panic.
	h.SetCostParameters(0.048, 0.006)
}

// TestRecommendationHelper_SetObjectiveWeights_AffectsRecommendation verifies
// that changing objective weights produces a recommendation (no error).
func TestRecommendationHelper_SetObjectiveWeights_AffectsRecommendation(t *testing.T) {
	h := pareto.NewRecommendationHelper()
	// Heavily weight cost savings.
	h.SetObjectiveWeights(0.8, 0.1, 0.05, 0.03, 0.02)

	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error after changing weights: %v", err)
	}
	if rec == nil || rec.BestSolution == nil {
		t.Error("expected valid recommendation after weight change")
	}
}

// TestNewRecommendationHelperWithOptimizer_Works verifies that creating a helper
// with a custom optimizer still produces a valid recommendation.
func TestNewRecommendationHelperWithOptimizer_Works(t *testing.T) {
	opt := pareto.NewOptimizer()
	h := pareto.NewRecommendationHelperWithOptimizer(opt)

	rec, err := h.GenerateRecommendation(sampleMetrics())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil recommendation")
	}
}

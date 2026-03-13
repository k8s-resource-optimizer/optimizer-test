package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/prediction"
)

// buildPodMetrics creates a slice of PodMetric entries with realistic seasonal
// CPU and memory usage over n hourly intervals.  Uses the same math as
// generatePredictorData so the Holt-Winters model can fit comfortably.
func buildPodMetrics(n int) []models.PodMetric {
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	metrics := make([]models.PodMetric, n)
	for i := range metrics {
		phase := 2 * 3.14159 * float64(i) / 24.0
		cpu := int64(500 + 200*sinApprox(phase) + float64(i)*0.5)
		mem := int64(256*1024*1024 + 50*1024*1024*sinApprox(phase))
		metrics[i] = models.PodMetric{
			PodName:   "api-abc123-xyz",
			Namespace: "production",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "api",
					UsageCPU:      cpu,
					UsageMemory:   mem,
					RequestCPU:    1000,
					RequestMemory: 512 * 1024 * 1024,
				},
			},
		}
	}
	return metrics
}

// sinApprox is a pure-Go sine approximation suitable for test data generation
// (avoids importing math in this file while keeping the helper self-contained).
func sinApprox(x float64) float64 {
	// Reduce to [-π, π]
	const pi = 3.14159265358979323846
	for x > pi {
		x -= 2 * pi
	}
	for x < -pi {
		x += 2 * pi
	}
	// Taylor series: sin(x) ≈ x - x³/6 + x⁵/120 - x⁷/5040
	x2 := x * x
	return x * (1 - x2/6*(1-x2/20*(1-x2/42)))
}

// ─── PredictWorkload tests ────────────────────────────────────────────────────

// TestWorkloadPredictor_PredictWorkload_InsufficientData verifies that
// PredictWorkload returns an error when given fewer data points than required.
func TestWorkloadPredictor_PredictWorkload_InsufficientData(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	metrics := buildPodMetrics(10) // far fewer than the 48-point minimum

	_, err := wp.PredictWorkload("production", "api", metrics)
	if err == nil {
		t.Error("expected error for insufficient data, got nil")
	}
}

// TestWorkloadPredictor_PredictWorkload_SufficientData verifies that
// PredictWorkload succeeds when given enough data points and returns a
// non-nil prediction with expected fields set.
func TestWorkloadPredictor_PredictWorkload_SufficientData(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	metrics := buildPodMetrics(72) // 3 days of hourly data — exceeds 48-point minimum

	pred, err := wp.PredictWorkload("production", "api", metrics)
	if err != nil {
		t.Fatalf("PredictWorkload error: %v", err)
	}
	if pred == nil {
		t.Fatal("expected non-nil prediction")
	}
	if pred.Namespace != "production" {
		t.Errorf("expected Namespace=production, got %q", pred.Namespace)
	}
	if pred.WorkloadName != "api" {
		t.Errorf("expected WorkloadName=api, got %q", pred.WorkloadName)
	}
	if pred.GeneratedAt.IsZero() {
		t.Error("expected GeneratedAt to be set")
	}
}

// TestWorkloadPredictor_PredictWorkload_PositiveCPURecommendation verifies
// that the resulting CPU recommendation is a positive value.
func TestWorkloadPredictor_PredictWorkload_PositiveCPURecommendation(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	metrics := buildPodMetrics(72)

	pred, err := wp.PredictWorkload("default", "worker", metrics)
	if err != nil {
		t.Fatalf("PredictWorkload error: %v", err)
	}
	if pred.RecommendedCPU < 0 {
		t.Errorf("expected non-negative RecommendedCPU, got %d", pred.RecommendedCPU)
	}
}

// TestWorkloadPredictor_PredictWorkload_ConfidenceInRange verifies that the
// confidence score is within the expected [0, 100] range.
func TestWorkloadPredictor_PredictWorkload_ConfidenceInRange(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	metrics := buildPodMetrics(72)

	pred, err := wp.PredictWorkload("default", "svc", metrics)
	if err != nil {
		t.Fatalf("PredictWorkload error: %v", err)
	}
	if pred.Confidence < 0 || pred.Confidence > 100 {
		t.Errorf("expected confidence in [0,100], got %.2f", pred.Confidence)
	}
}

// TestWorkloadPredictor_PredictWorkload_TrendDirectionValid verifies that
// CPUTrend and MemoryTrend are one of the known direction values.
func TestWorkloadPredictor_PredictWorkload_TrendDirectionValid(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	metrics := buildPodMetrics(72)

	pred, err := wp.PredictWorkload("default", "svc", metrics)
	if err != nil {
		t.Fatalf("PredictWorkload error: %v", err)
	}

	validDirections := map[prediction.TrendDirection]bool{
		prediction.TrendUp:     true,
		prediction.TrendDown:   true,
		prediction.TrendStable: true,
	}
	if !validDirections[pred.CPUTrend] {
		t.Errorf("unexpected CPUTrend value: %q", pred.CPUTrend)
	}
	if !validDirections[pred.MemoryTrend] {
		t.Errorf("unexpected MemoryTrend value: %q", pred.MemoryTrend)
	}
}

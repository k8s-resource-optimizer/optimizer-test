package integration_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// TestPredictionExtended_Smooth verifies the Smooth function.
func TestPredictionExtended_Smooth(t *testing.T) {
	data := []float64{1, 3, 5, 7, 9, 7, 5, 3, 1, 3, 5}
	smoothed := prediction.Smooth(data, 3)
	if len(smoothed) != len(data) {
		t.Errorf("expected same length %d, got %d", len(data), len(smoothed))
	}
	for _, v := range smoothed {
		if v < 0 {
			t.Error("expected non-negative smoothed values")
		}
	}
}

// TestPredictionExtended_MedianSmooth verifies the MedianSmooth function.
func TestPredictionExtended_MedianSmooth(t *testing.T) {
	data := []float64{1, 100, 2, 100, 3, 100, 4, 5, 6}
	smoothed := prediction.MedianSmooth(data, 3)
	if len(smoothed) != len(data) {
		t.Errorf("expected same length %d, got %d", len(data), len(smoothed))
	}
}

// TestPredictionExtended_HoltWintersGettersAfterFit verifies GetSeasonals and IsFitted.
func TestPredictionExtended_HoltWintersGettersAfterFit(t *testing.T) {
	hw := prediction.NewHoltWinters()
	if hw.IsFitted() {
		t.Error("expected IsFitted=false before fitting")
	}
	_ = hw.GetSeasonals() // may or may not be nil before fitting

	// Generate seasonal data and fit
	data := make([]float64, 72)
	for i := range data {
		phase := float64(i%24) / 24.0 * 6.283
		data[i] = 500 + 200*sinApprox(phase) + float64(i)*0.5
	}
	if err := hw.Fit(data); err != nil {
		t.Fatalf("HoltWinters Fit error: %v", err)
	}
	if !hw.IsFitted() {
		t.Error("expected IsFitted=true after fitting")
	}
	seasonals := hw.GetSeasonals()
	if seasonals == nil {
		t.Error("expected non-nil seasonals after fitting")
	}
}

// TestPredictionExtended_NewWorkloadPredictorWithConfig verifies custom config constructor.
func TestPredictionExtended_NewWorkloadPredictorWithConfig(t *testing.T) {
	cfg := prediction.DefaultConfig()
	wp := prediction.NewWorkloadPredictorWithConfig(cfg, 14, 1.2)
	if wp == nil {
		t.Fatal("expected non-nil WorkloadPredictor from NewWorkloadPredictorWithConfig")
	}
}

// sinApprox computes an approximation of sin(x) using Taylor series.
func sinApprox(x float64) float64 {
	// sin(x) ≈ x - x³/6 + x⁵/120 for small x
	// Normalize x to [-π, π]
	for x > 3.14159 {
		x -= 6.28318
	}
	for x < -3.14159 {
		x += 6.28318
	}
	x2 := x * x
	return x * (1 - x2/6*(1-x2/20))
}

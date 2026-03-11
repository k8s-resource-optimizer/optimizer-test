package unit_test

import (
	"math"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// generatePredictorData builds n (cpu, memory) float64 slices with a daily
// sinusoidal pattern, which gives Holt-Winters enough seasonal structure to fit.
func generatePredictorData(n int) (cpuVals, memVals []float64, ts []time.Time) {
	cpuVals = make([]float64, n)
	memVals = make([]float64, n)
	ts = make([]time.Time, n)
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	for i := range cpuVals {
		phase := 2 * math.Pi * float64(i) / 24.0 // daily period
		cpuVals[i] = 500 + 200*math.Sin(phase) + float64(i)*0.5
		memVals[i] = 256*1024*1024 + 50*1024*1024*math.Sin(phase)
		ts[i] = base.Add(time.Duration(i) * time.Hour)
	}
	return
}

// TestWorkloadPredictor_PredictFromValues_NoError verifies that prediction
// succeeds when given sufficient data (≥ MinDataPoints = 48).
func TestWorkloadPredictor_PredictFromValues_NoError(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72) // 3 days of hourly data

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}
	if pred == nil {
		t.Fatal("expected non-nil prediction")
	}
}

// TestWorkloadPredictor_PredictFromValues_InsufficientData verifies that
// the predictor returns an error when fewer than MinDataPoints are provided.
func TestWorkloadPredictor_PredictFromValues_InsufficientData(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(10) // way below MinDataPoints=48

	_, err := wp.PredictFromValues(cpu, mem, ts)
	if err == nil {
		t.Error("expected error for insufficient data, got nil")
	}
}

// TestWorkloadPredictor_PeakCPUPositive verifies that PeakCPU is positive
// after a successful prediction.
func TestWorkloadPredictor_PeakCPUPositive(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72)

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}
	if pred.PeakCPU <= 0 {
		t.Errorf("expected positive PeakCPU, got %f", pred.PeakCPU)
	}
}

// TestWorkloadPredictor_ConfidenceInRange verifies that the confidence score
// is within [0, 100].
func TestWorkloadPredictor_ConfidenceInRange(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72)

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}
	if pred.Confidence < 0 || pred.Confidence > 100 {
		t.Errorf("confidence %f is outside [0, 100]", pred.Confidence)
	}
}

// TestWorkloadPredictor_TrendDirectionKnown verifies that both CPUTrend and
// MemoryTrend are one of the three documented values.
func TestWorkloadPredictor_TrendDirectionKnown(t *testing.T) {
	valid := map[prediction.TrendDirection]bool{
		prediction.TrendUp:     true,
		prediction.TrendDown:   true,
		prediction.TrendStable: true,
	}

	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72)

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}
	if !valid[pred.CPUTrend] {
		t.Errorf("unknown CPUTrend %q", pred.CPUTrend)
	}
	if !valid[pred.MemoryTrend] {
		t.Errorf("unknown MemoryTrend %q", pred.MemoryTrend)
	}
}

// TestWorkloadPredictor_ShouldScaleUp_WhenPeakExceedsCurrent verifies that
// ShouldScaleUp returns true when predicted peak exceeds current allocation.
func TestWorkloadPredictor_ShouldScaleUp_WhenPeakExceedsCurrent(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72)

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}

	// Set current allocation well below the predicted peak.
	lowCPU := int64(pred.PeakCPU * 0.3)
	lowMem := int64(pred.PeakMemory * 0.3)

	if !pred.ShouldScaleUp(lowCPU, lowMem) {
		t.Error("expected ShouldScaleUp=true when peak >> current allocation")
	}
}

// TestWorkloadPredictor_ShouldScaleDown_WhenCurrentFarExceedsPeak verifies
// that ShouldScaleDown returns true when current is much larger than peak.
func TestWorkloadPredictor_ShouldScaleDown_WhenCurrentFarExceedsPeak(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72)

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}

	// Set current allocation 10× the predicted peak.
	hugeCPU := int64(pred.PeakCPU * 10)
	hugeMem := int64(pred.PeakMemory * 10)

	if !pred.ShouldScaleDown(hugeCPU, hugeMem, 0.5) {
		t.Error("expected ShouldScaleDown=true when current >> peak")
	}
}

// TestWorkloadPredictor_Summary_NotEmpty verifies that Summary returns a
// non-empty descriptive string.
func TestWorkloadPredictor_Summary_NotEmpty(t *testing.T) {
	wp := prediction.NewWorkloadPredictor()
	cpu, mem, ts := generatePredictorData(72)

	pred, err := wp.PredictFromValues(cpu, mem, ts)
	if err != nil {
		t.Fatalf("PredictFromValues: %v", err)
	}
	if pred.Summary() == "" {
		t.Error("Summary() should not be empty")
	}
}

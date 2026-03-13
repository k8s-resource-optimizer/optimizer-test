package unit_test

import (
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// makePrediction constructs a WorkloadPrediction with known values that are
// useful for exercising GetScalingRecommendation and TimeUntilPeak.
func makePrediction() *prediction.WorkloadPrediction {
	return &prediction.WorkloadPrediction{
		Namespace:         "default",
		WorkloadName:      "my-app",
		PeakCPU:           500,
		PeakMemory:        256 * 1024 * 1024,
		RecommendedCPU:    600,
		RecommendedMemory: 300 * 1024 * 1024,
		PeakCPUTime:       time.Now().Add(2 * time.Hour),
		PeakMemoryTime:    time.Now().Add(3 * time.Hour),
		GeneratedAt:       time.Now(),
	}
}

// TestWorkloadPredictor_NewWithConfig_NonNil verifies that
// NewWorkloadPredictorWithConfig returns a non-nil predictor.
func TestWorkloadPredictor_NewWithConfig_NonNil(t *testing.T) {
	cfg := prediction.DefaultConfig()
	cfg.SeasonalPeriod = 12
	wp := prediction.NewWorkloadPredictorWithConfig(cfg, 48, 1.3)
	if wp == nil {
		t.Fatal("expected non-nil WorkloadPredictor")
	}
}

// TestWorkloadPredictor_NewWithConfig_HorizonPreserved verifies that the
// ForecastHorizon field is set to the provided value.
func TestWorkloadPredictor_NewWithConfig_HorizonPreserved(t *testing.T) {
	cfg := prediction.DefaultConfig()
	const horizon = 36
	wp := prediction.NewWorkloadPredictorWithConfig(cfg, horizon, 1.1)
	if wp.ForecastHorizon != horizon {
		t.Errorf("expected ForecastHorizon=%d, got %d", horizon, wp.ForecastHorizon)
	}
}

// TestWorkloadPredictor_NewWithConfig_SafetyMarginPreserved verifies that the
// SafetyMargin field is set to the provided value.
func TestWorkloadPredictor_NewWithConfig_SafetyMarginPreserved(t *testing.T) {
	cfg := prediction.DefaultConfig()
	const margin = 1.5
	wp := prediction.NewWorkloadPredictorWithConfig(cfg, 24, margin)
	if wp.SafetyMargin != margin {
		t.Errorf("expected SafetyMargin=%f, got %f", margin, wp.SafetyMargin)
	}
}

// TestGetScalingRecommendation_ScaleUp verifies that when predicted peak
// exceeds current resources the recommendation contains "SCALE UP".
func TestGetScalingRecommendation_ScaleUp(t *testing.T) {
	pred := makePrediction()
	// Set current resources well below the recommended (peak-based) values.
	currentCPU := int64(100)                 // far below RecommendedCPU=600
	currentMem := int64(64 * 1024 * 1024)   // far below RecommendedMemory
	rec := pred.GetScalingRecommendation(currentCPU, currentMem)
	if !strings.Contains(rec, "SCALE UP") {
		t.Errorf("expected 'SCALE UP' recommendation, got: %s", rec)
	}
}

// TestGetScalingRecommendation_ScaleDown verifies that when current resources
// greatly exceed the predicted peak the recommendation contains "SCALE DOWN".
func TestGetScalingRecommendation_ScaleDown(t *testing.T) {
	pred := makePrediction()
	// Set current resources 10× the predicted peak — clearly over-provisioned.
	currentCPU := int64(pred.PeakCPU * 10)
	currentMem := int64(pred.PeakMemory * 10)
	rec := pred.GetScalingRecommendation(currentCPU, currentMem)
	if !strings.Contains(rec, "SCALE DOWN") {
		t.Errorf("expected 'SCALE DOWN' recommendation, got: %s", rec)
	}
}

// TestGetScalingRecommendation_Maintain verifies that when current resources
// are roughly aligned with the predicted peak the recommendation contains
// "MAINTAIN".
func TestGetScalingRecommendation_Maintain(t *testing.T) {
	pred := makePrediction()
	// Current resources are exactly the recommended values — balanced.
	currentCPU := pred.RecommendedCPU
	currentMem := pred.RecommendedMemory
	rec := pred.GetScalingRecommendation(currentCPU, currentMem)
	if !strings.Contains(rec, "MAINTAIN") {
		t.Errorf("expected 'MAINTAIN' recommendation, got: %s", rec)
	}
}

// TestTimeUntilPeak_CPU_Positive verifies that when PeakCPUTime is set to a
// future time the returned CPU duration is positive.
func TestTimeUntilPeak_CPU_Positive(t *testing.T) {
	pred := makePrediction() // PeakCPUTime = now + 2h
	cpuDur, _ := pred.TimeUntilPeak()
	if cpuDur <= 0 {
		t.Errorf("expected positive cpu duration, got %v", cpuDur)
	}
}

// TestTimeUntilPeak_Memory_Positive verifies that when PeakMemoryTime is set
// to a future time the returned memory duration is positive.
func TestTimeUntilPeak_Memory_Positive(t *testing.T) {
	pred := makePrediction() // PeakMemoryTime = now + 3h
	_, memDur := pred.TimeUntilPeak()
	if memDur <= 0 {
		t.Errorf("expected positive memory duration, got %v", memDur)
	}
}

// TestTimeUntilPeak_ZeroPeakTime_ReturnZero verifies that a zero PeakCPUTime
// causes the cpu duration to be returned as zero.
func TestTimeUntilPeak_ZeroPeakTime_ReturnZero(t *testing.T) {
	pred := makePrediction()
	pred.PeakCPUTime = time.Time{} // zero value
	cpuDur, _ := pred.TimeUntilPeak()
	if cpuDur != 0 {
		t.Errorf("expected zero cpu duration for zero PeakCPUTime, got %v", cpuDur)
	}
}

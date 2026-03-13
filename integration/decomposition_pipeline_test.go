package integration_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// TestDecomposition_SimpleDecompose verifies simpleDecompose via short series.
func TestDecomposition_SimpleDecompose(t *testing.T) {
	// With period=24 and only 10 data points, simpleDecompose path is triggered (n < 2*period)
	d := prediction.NewDecomposer(24)
	data := []float64{100, 110, 120, 130, 140, 135, 125, 115, 105, 100}

	result, err := d.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose (simpleDecompose path) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil DecompositionResult")
	}
	if len(result.Trend) != len(data) {
		t.Errorf("expected trend len %d, got %d", len(data), len(result.Trend))
	}
}

// TestDecomposition_FullDecompose verifies the full decomposition path with enough data.
func TestDecomposition_FullDecompose(t *testing.T) {
	period := 12
	n := 60 // 5 periods
	d := prediction.NewDecomposer(period)

	data := make([]float64, n)
	for i := 0; i < n; i++ {
		seasonal := sinApprox(float64(i%period) / float64(period) * 6.28318)
		data[i] = 100 + float64(i)*0.5 + 20*seasonal
	}

	result, err := d.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose (full path) error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil DecompositionResult")
	}

	trendStrength, seasonalStrength := result.GetStrength()
	if trendStrength < 0 || trendStrength > 1 {
		t.Errorf("trend strength out of [0,1]: %f", trendStrength)
	}
	if seasonalStrength < 0 || seasonalStrength > 1 {
		t.Errorf("seasonal strength out of [0,1]: %f", seasonalStrength)
	}
}

// TestDecomposition_DetectSeasonalPeriod verifies DetectSeasonalPeriod.
func TestDecomposition_DetectSeasonalPeriod(t *testing.T) {
	n := 72
	data := make([]float64, n)
	for i := 0; i < n; i++ {
		data[i] = 100 + 50*sinApprox(float64(i%24)/24.0*6.28318)
	}

	period := prediction.DetectSeasonalPeriod(data, 48)
	if period <= 0 {
		t.Errorf("expected positive period, got %d", period)
	}
}

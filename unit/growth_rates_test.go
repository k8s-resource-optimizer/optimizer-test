package unit_test

import (
	"math"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/trends"
)

// TestCalculateGrowthRates_DailyPositiveForGrowth verifies a positive daily
// growth rate for a linearly growing series (already in trends_test.go for
// the basic case; here we also check weekly/monthly fields).
func TestCalculateGrowthRates_AllFieldsPresent(t *testing.T) {
	data := genLinear(100, 100, 2)
	ts := genTimestamps(100, time.Hour)

	rates := trends.CalculateGrowthRates(data, ts)

	// All rates should be finite (not NaN/Inf).
	for _, v := range []float64{rates.Daily, rates.Weekly, rates.Monthly} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("growth rate field is NaN or Inf: daily=%f weekly=%f monthly=%f",
				rates.Daily, rates.Weekly, rates.Monthly)
		}
	}
}

// TestCalculateGrowthRates_StableSeriesNearZero verifies that a constant
// series produces growth rates very close to zero.
func TestCalculateGrowthRates_StableSeriesNearZero(t *testing.T) {
	data := make([]float64, 100)
	for i := range data {
		data[i] = 500
	}
	ts := genTimestamps(100, time.Hour)

	rates := trends.CalculateGrowthRates(data, ts)

	// Allow a tiny floating-point margin.
	if math.Abs(rates.Daily) > 1e-6 {
		t.Errorf("stable series: expected daily ≈ 0, got %f", rates.Daily)
	}
}

// TestCalculateGrowthRates_WeeklyGreaterThanDaily verifies that for a growing
// series, the weekly rate is larger than the daily rate.
func TestCalculateGrowthRates_WeeklyGreaterThanDaily(t *testing.T) {
	data := genLinear(100, 100, 2)
	ts := genTimestamps(100, time.Hour)

	rates := trends.CalculateGrowthRates(data, ts)

	if rates.Weekly <= rates.Daily {
		t.Errorf("expected weekly (%f) > daily (%f) for growing series", rates.Weekly, rates.Daily)
	}
}

// TestCalculateCAGR_PositiveForGrowingData verifies that CAGR is positive
// for a series that grows from start to finish.
func TestCalculateCAGR_PositiveForGrowingData(t *testing.T) {
	data := genLinear(100, 100, 2) // 100 → 298
	cagr := trends.CalculateCAGR(data, len(data))
	if cagr <= 0 {
		t.Errorf("expected positive CAGR for growing series, got %f", cagr)
	}
}

// TestCalculateCAGR_ZeroForFlatData verifies that CAGR is zero (or very near
// zero) for a flat series.
func TestCalculateCAGR_ZeroForFlatData(t *testing.T) {
	data := make([]float64, 100)
	for i := range data {
		data[i] = 200
	}
	cagr := trends.CalculateCAGR(data, len(data))
	if math.Abs(cagr) > 1e-6 {
		t.Errorf("expected CAGR ≈ 0 for flat series, got %f", cagr)
	}
}

// TestCalculateAcceleration_PositiveForAcceleratingGrowth verifies that a
// series with increasing slope (quadratic) has positive acceleration.
func TestCalculateAcceleration_PositiveForAcceleratingGrowth(t *testing.T) {
	// Quadratic: y = i^2 — second derivative is positive constant.
	n := 50
	data := make([]float64, n)
	for i := range data {
		data[i] = float64(i * i)
	}
	acc := trends.CalculateAcceleration(data)
	if acc <= 0 {
		t.Errorf("expected positive acceleration for quadratic growth, got %f", acc)
	}
}

// TestCalculateAcceleration_NearZeroForLinear verifies that a linear series
// has near-zero acceleration (constant slope, no change in growth rate).
func TestCalculateAcceleration_NearZeroForLinear(t *testing.T) {
	data := genLinear(100, 0, 3) // perfectly linear
	acc := trends.CalculateAcceleration(data)
	if math.Abs(acc) > 1.0 { // allow small floating-point noise
		t.Errorf("expected near-zero acceleration for linear series, got %f", acc)
	}
}

// TestCalculateAcceleration_NegativeForDeceleratingGrowth verifies that a
// series slowing down (square root) has negative acceleration.
func TestCalculateAcceleration_NegativeForDeceleratingGrowth(t *testing.T) {
	// sqrt(i) grows fast at first then slows — negative second derivative.
	n := 100
	data := make([]float64, n)
	for i := range data {
		data[i] = math.Sqrt(float64(i + 1))
	}
	acc := trends.CalculateAcceleration(data)
	if acc >= 0 {
		t.Errorf("expected negative acceleration for decelerating growth (sqrt), got %f", acc)
	}
}

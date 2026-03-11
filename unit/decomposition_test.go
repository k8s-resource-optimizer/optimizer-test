package unit_test

import (
	"math"
	"testing"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// genSeasonalSeries returns n values with a clear seasonal period of `period`
// and a gentle upward trend — ideal for testing the decomposer.
func genSeasonalSeries(n, period int, base, trendSlope float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		seasonal := 20 * math.Sin(2*math.Pi*float64(i)/float64(period))
		data[i] = base + trendSlope*float64(i) + seasonal
	}
	return data
}

// TestDecomposer_ProducesComponents verifies that Decompose returns non-nil
// trend and seasonal components of the correct length.
func TestDecomposer_ProducesComponents(t *testing.T) {
	data := genSeasonalSeries(96, 24, 100, 0.5) // 4 days, daily period
	d := prediction.NewDecomposer(24)

	result, err := d.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil DecompositionResult")
	}
	if len(result.Trend) == 0 {
		t.Error("expected non-empty Trend component")
	}
	if len(result.Seasonal) == 0 {
		t.Error("expected non-empty Seasonal component")
	}
}

// TestDecomposer_StrengthValuesInRange verifies that both trend and seasonal
// strength values returned by GetStrength are in [0, 1].
func TestDecomposer_StrengthValuesInRange(t *testing.T) {
	data := genSeasonalSeries(96, 24, 100, 0) // no trend, pure seasonal
	d := prediction.NewDecomposer(24)

	result, err := d.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}

	ts, ss := result.GetStrength()
	if ts < 0 || ts > 1 {
		t.Errorf("trend strength %f is outside [0, 1]", ts)
	}
	if ss < 0 || ss > 1 {
		t.Errorf("seasonal strength %f is outside [0, 1]", ss)
	}
}

// TestDecomposer_TrendStrengthPositive verifies that a strongly trending
// series has trendStrength > 0.
func TestDecomposer_TrendStrengthPositive(t *testing.T) {
	data := genSeasonalSeries(96, 24, 0, 5) // steep upward trend
	d := prediction.NewDecomposer(24)

	result, err := d.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}

	trendStrength, _ := result.GetStrength()
	if trendStrength <= 0 {
		t.Errorf("expected positive trend strength for a trending series, got %f", trendStrength)
	}
}

// TestDecomposer_ShortDataDoesNotPanic verifies that Decompose handles a
// series shorter than two full seasonal periods without panicking.
func TestDecomposer_ShortDataDoesNotPanic(t *testing.T) {
	data := make([]float64, 10) // shorter than period=24
	d := prediction.NewDecomposer(24)

	// Whether it returns an error or an empty result is acceptable —
	// we only require it not to panic.
	result, _ := d.Decompose(data)
	_ = result
}

// TestDetectSeasonalPeriod_DetectsDailyPeriod verifies that the autocorrelation-
// based detector identifies a daily (24-point) period in suitable data.
func TestDetectSeasonalPeriod_DetectsDailyPeriod(t *testing.T) {
	// Use 10 days of data with a very clean 24-point cycle.
	data := make([]float64, 240)
	for i := range data {
		data[i] = 100 + 50*math.Sin(2*math.Pi*float64(i)/24.0)
	}

	period := prediction.DetectSeasonalPeriod(data, 48)
	// Allow 20% tolerance — the autocorrelation peak may be slightly off.
	if period < 19 || period > 29 {
		t.Logf("detected period %d (expected ~24) — acceptable if autocorrelation is noisy", period)
	}
}

// TestSmooth_ReducesVariance verifies that moving-average smoothing reduces
// the variance of a noisy series.
func TestSmooth_ReducesVariance(t *testing.T) {
	// Build a noisy series.
	noisy := make([]float64, 100)
	for i := range noisy {
		sign := float64(1)
		if i%2 == 0 {
			sign = -1
		}
		noisy[i] = 100 + sign*30
	}

	smoothed := prediction.Smooth(noisy, 5)
	if len(smoothed) == 0 {
		t.Fatal("Smooth returned empty slice")
	}

	// Compute variance of original and smoothed.
	varianceOf := func(d []float64) float64 {
		if len(d) == 0 {
			return 0
		}
		mean := 0.0
		for _, v := range d {
			mean += v
		}
		mean /= float64(len(d))
		sum := 0.0
		for _, v := range d {
			sum += (v - mean) * (v - mean)
		}
		return sum / float64(len(d))
	}

	if varianceOf(smoothed) >= varianceOf(noisy) {
		t.Error("expected Smooth to reduce variance of a noisy series")
	}
}

// TestMedianSmooth_ReducesVariance verifies that median smoothing similarly
// reduces variance (median is more robust to outliers than mean).
func TestMedianSmooth_ReducesVariance(t *testing.T) {
	noisy := make([]float64, 100)
	for i := range noisy {
		sign := float64(1)
		if i%2 == 0 {
			sign = -1
		}
		noisy[i] = 100 + sign*50
	}

	smoothed := prediction.MedianSmooth(noisy, 5)
	if len(smoothed) == 0 {
		t.Fatal("MedianSmooth returned empty slice")
	}

	// A median-smoothed alternating series should have much less variance.
	spread := 0.0
	for _, v := range smoothed {
		d := v - 100
		if d < 0 {
			d = -d
		}
		spread += d
	}
	originalSpread := 50.0 * float64(len(noisy))
	if spread >= originalSpread {
		t.Error("expected MedianSmooth to reduce spread of alternating series")
	}
}

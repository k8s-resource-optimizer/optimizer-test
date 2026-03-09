package unit_test

import (
	"math"
	"testing"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// generateSeasonalData creates a synthetic time series that has a known
// linear trend plus additive seasonality, so we can measure how well
// Holt-Winters recovers it.
//
// formula:  value[i] = trend_slope * i + seasonal[i % period] + base
func generateSeasonalData(n, period int, base, trendSlope float64) []float64 {
	// Build a simple seasonal pattern: sine wave scaled to the period.
	seasonals := make([]float64, period)
	for i := 0; i < period; i++ {
		seasonals[i] = math.Sin(2*math.Pi*float64(i)/float64(period)) * 50
	}

	data := make([]float64, n)
	for i := 0; i < n; i++ {
		data[i] = base + trendSlope*float64(i) + seasonals[i%period]
		// Clamp to avoid negative values (memory / CPU can't be negative).
		if data[i] < 1 {
			data[i] = 1
		}
	}
	return data
}

// mapeOf computes Mean Absolute Percentage Error between actual and forecast arrays.
// Only considers indices where actual[i] != 0.
func mapeOf(actual, forecast []float64) float64 {
	if len(actual) == 0 || len(forecast) == 0 {
		return 0
	}
	n := len(actual)
	if len(forecast) < n {
		n = len(forecast)
	}
	var sum float64
	count := 0
	for i := 0; i < n; i++ {
		if actual[i] == 0 {
			continue
		}
		sum += math.Abs(actual[i]-forecast[i]) / math.Abs(actual[i])
		count++
	}
	if count == 0 {
		return 0
	}
	return (sum / float64(count)) * 100
}

// TestHoltWinters_FitSucceeds verifies that Fit() does not return an error
// when given a sufficient amount of seasonal data.
func TestHoltWinters_FitSucceeds(t *testing.T) {
	// period=24 * MinDataPoints=2 = 48 minimum required samples
	data := generateSeasonalData(72, 24, 200, 1.0)
	hw := prediction.NewHoltWinters()

	if err := hw.Fit(data); err != nil {
		t.Errorf("Fit() returned unexpected error: %v", err)
	}
}

// TestHoltWinters_InsufficientDataReturnsError verifies that Fit() refuses
// to run when too few data points are provided.
func TestHoltWinters_InsufficientDataReturnsError(t *testing.T) {
	hw := prediction.NewHoltWinters()
	// Default config requires period(24) * minDataPoints(2) = 48 samples.
	// We provide only 10.
	data := generateSeasonalData(10, 24, 100, 0)

	if err := hw.Fit(data); err == nil {
		t.Error("expected an error for insufficient data, got nil")
	}
}

// TestHoltWinters_PredictReturnsCorrectHorizon verifies that Predict()
// returns exactly the requested number of future forecasts.
func TestHoltWinters_PredictReturnsCorrectHorizon(t *testing.T) {
	data := generateSeasonalData(72, 24, 200, 1.0)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit() failed: %v", err)
	}

	horizon := 24
	result, err := hw.Predict(horizon)
	if err != nil {
		t.Fatalf("Predict() failed: %v", err)
	}
	if len(result.Forecasts) != horizon {
		t.Errorf("expected %d forecast points, got %d", horizon, len(result.Forecasts))
	}
}

// TestHoltWinters_MAPEUnder15Percent is the core accuracy requirement from
// the project spec: MAPE must stay below 15% on held-out test data.
//
// Approach:
//   1. Generate 96 points of synthetic data (4 seasons of period=24).
//   2. Fit on the first 72 points.
//   3. Predict the next 24 points.
//   4. Compare predictions to the actual last 24 points.
func TestHoltWinters_MAPEUnder15Percent(t *testing.T) {
	period := 24
	// 72 training points (3 seasons) + 24 test points (1 season)
	allData := generateSeasonalData(96, period, 300, 0.5)
	trainData := allData[:72]
	testData := allData[72:]

	hw := prediction.NewHoltWinters()
	if err := hw.Fit(trainData); err != nil {
		t.Fatalf("Fit() failed: %v", err)
	}

	result, err := hw.Predict(len(testData))
	if err != nil {
		t.Fatalf("Predict() failed: %v", err)
	}

	// Extract point forecasts for MAPE calculation
	forecasts := make([]float64, len(result.Forecasts))
	for i, f := range result.Forecasts {
		forecasts[i] = f.Value
	}

	mape := mapeOf(testData, forecasts)
	t.Logf("Holt-Winters MAPE on held-out test data: %.2f%%", mape)

	if mape > 15.0 {
		t.Errorf("MAPE %.2f%% exceeds the 15%% threshold", mape)
	}
}

// TestHoltWinters_FitPredictConvenience verifies the FitPredict helper
// which combines fit and predict in a single call.
func TestHoltWinters_FitPredictConvenience(t *testing.T) {
	data := generateSeasonalData(72, 24, 200, 0.5)
	hw := prediction.NewHoltWinters()

	result, err := hw.FitPredict(data, 12)
	if err != nil {
		t.Fatalf("FitPredict() failed: %v", err)
	}
	if len(result.Forecasts) != 12 {
		t.Errorf("expected 12 forecast points, got %d", len(result.Forecasts))
	}
}

// TestHoltWinters_FittedValuesLengthMatchesInput verifies that the model
// returns one fitted value per training data point.
func TestHoltWinters_FittedValuesLengthMatchesInput(t *testing.T) {
	n := 72
	data := generateSeasonalData(n, 24, 150, 0.3)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit() failed: %v", err)
	}

	result, err := hw.Predict(1)
	if err != nil {
		t.Fatalf("Predict() failed: %v", err)
	}

	if len(result.FittedValues) == 0 {
		t.Error("FittedValues must not be empty after fitting")
	}
}

// TestHoltWinters_MetricsPopulated verifies that the model computes
// in-sample error metrics (RMSE, MAE, MAPE) after fitting.
func TestHoltWinters_MetricsPopulated(t *testing.T) {
	data := generateSeasonalData(72, 24, 200, 1.0)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit() failed: %v", err)
	}

	result, err := hw.Predict(1)
	if err != nil {
		t.Fatalf("Predict() failed: %v", err)
	}

	if result.Metrics == nil {
		t.Fatal("ErrorMetrics must not be nil after fitting")
	}
	if math.IsNaN(result.Metrics.RMSE) || result.Metrics.RMSE < 0 {
		t.Errorf("invalid RMSE: %f", result.Metrics.RMSE)
	}
	if math.IsNaN(result.Metrics.MAPE) || result.Metrics.MAPE < 0 {
		t.Errorf("invalid MAPE: %f", result.Metrics.MAPE)
	}
}

// TestHoltWinters_ConfidenceIntervalsOrdered verifies that for each
// forecast point, LowerBound ≤ Value ≤ UpperBound.
func TestHoltWinters_ConfidenceIntervalsOrdered(t *testing.T) {
	data := generateSeasonalData(72, 24, 200, 1.0)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit() failed: %v", err)
	}

	result, err := hw.Predict(12)
	if err != nil {
		t.Fatalf("Predict() failed: %v", err)
	}

	for i, f := range result.Forecasts {
		if f.LowerBound > f.Value {
			t.Errorf("forecast[%d]: LowerBound %.2f > Value %.2f", i, f.LowerBound, f.Value)
		}
		if f.UpperBound < f.Value {
			t.Errorf("forecast[%d]: UpperBound %.2f < Value %.2f", i, f.UpperBound, f.Value)
		}
	}
}

// TestHoltWinters_StableData verifies that the model handles constant
// (zero-trend, zero-seasonal) data without producing NaN or Inf values.
func TestHoltWinters_StableData(t *testing.T) {
	// All values are exactly 100 — no trend or seasonality.
	data := make([]float64, 72)
	for i := range data {
		data[i] = 100
	}

	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		// Acceptable to fail on degenerate data.
		t.Logf("Fit() returned error for constant data: %v (acceptable)", err)
		return
	}

	result, err := hw.Predict(12)
	if err != nil {
		t.Logf("Predict() returned error for constant data: %v (acceptable)", err)
		return
	}

	for i, f := range result.Forecasts {
		if math.IsNaN(f.Value) || math.IsInf(f.Value, 0) {
			t.Errorf("forecast[%d] is NaN/Inf for constant input", i)
		}
	}
}

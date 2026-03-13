package unit_test

import (
	"math"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/sla"
)

// makeMetrics returns a slice of n evenly-valued sla.Metric entries.
func makeMetrics(value float64, n int) []sla.Metric {
	metrics := make([]sla.Metric, n)
	for i := range metrics {
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Value:     value,
		}
	}
	return metrics
}

// makeLinearMetrics returns metrics with values that increase linearly by step.
func makeLinearMetrics(start, step float64, n int) []sla.Metric {
	metrics := make([]sla.Metric, n)
	for i := range metrics {
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Value:     start + float64(i)*step,
		}
	}
	return metrics
}

// ─── CalculateControlLimits tests ───────────────────────────────────────────

// TestControlChart_CalculateControlLimits_EmptyReturnsError verifies that an
// empty metric slice returns an error.
func TestControlChart_CalculateControlLimits_EmptyReturnsError(t *testing.T) {
	cc := sla.NewControlChart()
	_, _, _, err := cc.CalculateControlLimits([]sla.Metric{}, 3.0)
	if err == nil {
		t.Error("expected error for empty metrics, got nil")
	}
}

// TestControlChart_CalculateControlLimits_FlatMeanEqualsValue verifies that
// for a flat series the mean equals the constant value.
func TestControlChart_CalculateControlLimits_FlatMeanEqualsValue(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeMetrics(100.0, 20)
	mean, _, _, err := cc.CalculateControlLimits(metrics, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(mean-100.0) > 1e-9 {
		t.Errorf("expected mean=100.0 for flat series, got %.6f", mean)
	}
}

// TestControlChart_CalculateControlLimits_UCLAboveMean verifies UCL > mean.
func TestControlChart_CalculateControlLimits_UCLAboveMean(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeLinearMetrics(50, 5, 20)
	mean, ucl, _, err := cc.CalculateControlLimits(metrics, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ucl <= mean {
		t.Errorf("expected UCL (%.2f) > mean (%.2f)", ucl, mean)
	}
}

// TestControlChart_CalculateControlLimits_LCLNotNegative verifies LCL >= 0
// because the implementation clamps negative lower bounds.
func TestControlChart_CalculateControlLimits_LCLNotNegative(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeMetrics(10.0, 10)
	_, _, lcl, err := cc.CalculateControlLimits(metrics, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lcl < 0 {
		t.Errorf("expected LCL >= 0, got %.6f", lcl)
	}
}

// ─── GenerateChart tests ─────────────────────────────────────────────────────

// TestControlChart_GenerateChart_InsufficientSamplesErrors verifies that fewer
// samples than MinSamples returns an error.
func TestControlChart_GenerateChart_InsufficientSamplesErrors(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeMetrics(50.0, 3)
	config := sla.ControlChartConfig{MinSamples: 10, SigmaLevel: 3.0}
	_, err := cc.GenerateChart(metrics, config)
	if err == nil {
		t.Error("expected error for insufficient samples, got nil")
	}
}

// TestControlChart_GenerateChart_CorrectPointCount verifies that the returned
// slice has the same length as the input metrics.
func TestControlChart_GenerateChart_CorrectPointCount(t *testing.T) {
	cc := sla.NewControlChart()
	n := 20
	metrics := makeMetrics(100.0, n)
	config := sla.ControlChartConfig{MinSamples: 5, SigmaLevel: 3.0}
	points, err := cc.GenerateChart(metrics, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != n {
		t.Errorf("expected %d points, got %d", n, len(points))
	}
}

// TestControlChart_GenerateChart_FlatSeriesNoOutliers verifies that a perfectly
// flat series produces no outlier points.
func TestControlChart_GenerateChart_FlatSeriesNoOutliers(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeMetrics(50.0, 20)
	config := sla.ControlChartConfig{MinSamples: 5, SigmaLevel: 3.0}
	points, err := cc.GenerateChart(metrics, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range points {
		if p.IsOutlier {
			t.Errorf("expected no outliers in flat series, found one at value %.2f", p.Value)
			break
		}
	}
}

// TestControlChart_GenerateChart_OutlierDetected verifies that an extreme spike
// in an otherwise flat series is marked as an outlier.
func TestControlChart_GenerateChart_OutlierDetected(t *testing.T) {
	cc := sla.NewControlChart()
	// 19 normal + 1 extreme outlier
	metrics := makeMetrics(50.0, 19)
	metrics = append(metrics, sla.Metric{
		Timestamp: time.Now().Add(20 * time.Minute),
		Value:     9999.0, // extreme spike
	})
	config := sla.ControlChartConfig{MinSamples: 5, SigmaLevel: 3.0}
	points, err := cc.GenerateChart(metrics, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, p := range points {
		if p.IsOutlier {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one outlier for extreme spike value")
	}
}

// TestControlChart_GenerateChart_TrendDetection verifies that enabling trend
// detection with an increasing series produces at least one trend-flagged point.
func TestControlChart_GenerateChart_TrendDetection(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeLinearMetrics(10, 10, 20)
	config := sla.ControlChartConfig{
		MinSamples:           5,
		SigmaLevel:           3.0,
		EnableTrendDetection: true,
		TrendWindowSize:      5,
	}
	points, err := cc.GenerateChart(metrics, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, p := range points {
		if p.IsOutlier {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected trend to flag at least one point in a monotonically increasing series")
	}
}

// ─── DetectOutliers tests ────────────────────────────────────────────────────

// TestControlChart_DetectOutliers_EmptyReturnsNil verifies that an empty metric
// slice returns nil without error.
func TestControlChart_DetectOutliers_EmptyReturnsNil(t *testing.T) {
	cc := sla.NewControlChart()
	result, err := cc.DetectOutliers([]sla.Metric{}, 3.0)
	if err != nil {
		t.Errorf("expected no error for empty metrics, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty metrics, got %v", result)
	}
}

// TestControlChart_DetectOutliers_FlatSeriesNoOutliers verifies that a flat
// series has no detected outliers.
func TestControlChart_DetectOutliers_FlatSeriesNoOutliers(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeMetrics(100.0, 20)
	outliers, err := cc.DetectOutliers(metrics, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outliers) != 0 {
		t.Errorf("expected 0 outliers for flat series, got %d", len(outliers))
	}
}

// TestControlChart_DetectOutliers_ExtremeValueIsOutlier verifies that a single
// extreme value among otherwise normal values is detected as an outlier.
func TestControlChart_DetectOutliers_ExtremeValueIsOutlier(t *testing.T) {
	cc := sla.NewControlChart()
	metrics := makeMetrics(50.0, 19)
	metrics = append(metrics, sla.Metric{
		Timestamp: time.Now().Add(20 * time.Minute),
		Value:     50000.0,
	})
	outliers, err := cc.DetectOutliers(metrics, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outliers) == 0 {
		t.Error("expected at least one outlier for extreme spike")
	}
}

// ─── CalculateMovingAverage tests ───────────────────────────────────────────

// TestCalculateMovingAverage_ResultLengthMatchesInput verifies that the
// returned slice has the same length as the input.
func TestCalculateMovingAverage_ResultLengthMatchesInput(t *testing.T) {
	metrics := makeMetrics(100.0, 15)
	result := sla.CalculateMovingAverage(metrics, 5)
	if len(result) != len(metrics) {
		t.Errorf("expected length %d, got %d", len(metrics), len(result))
	}
}

// TestCalculateMovingAverage_FlatSeriesReturnsSameValue verifies that a flat
// series produces a moving average equal to the constant value throughout.
func TestCalculateMovingAverage_FlatSeriesReturnsSameValue(t *testing.T) {
	metrics := makeMetrics(200.0, 10)
	result := sla.CalculateMovingAverage(metrics, 3)
	for i, v := range result {
		if math.Abs(v-200.0) > 1e-9 {
			t.Errorf("index %d: expected moving average=200.0 for flat series, got %.6f", i, v)
		}
	}
}

// TestCalculateMovingAverage_ZeroWindowUsesFullSlice verifies that a zero
// window size falls back gracefully (no panic).
func TestCalculateMovingAverage_ZeroWindowUsesFullSlice(t *testing.T) {
	metrics := makeMetrics(50.0, 10)
	result := sla.CalculateMovingAverage(metrics, 0)
	if len(result) != len(metrics) {
		t.Errorf("expected result length %d, got %d", len(metrics), len(result))
	}
}

// ─── CalculateStandardDeviation tests ───────────────────────────────────────

// TestCalculateStandardDeviation_EmptyReturnsZero verifies that an empty slice
// returns 0.
func TestCalculateStandardDeviation_EmptyReturnsZero(t *testing.T) {
	sd := sla.CalculateStandardDeviation([]sla.Metric{})
	if sd != 0 {
		t.Errorf("expected 0 for empty metrics, got %f", sd)
	}
}

// TestCalculateStandardDeviation_FlatSeriesReturnsZero verifies that a flat
// series has standard deviation of 0.
func TestCalculateStandardDeviation_FlatSeriesReturnsZero(t *testing.T) {
	metrics := makeMetrics(42.0, 20)
	sd := sla.CalculateStandardDeviation(metrics)
	if math.Abs(sd) > 1e-9 {
		t.Errorf("expected stddev=0 for flat series, got %f", sd)
	}
}

// TestCalculateStandardDeviation_PositiveForVariableData verifies that a
// variable series has a positive standard deviation.
func TestCalculateStandardDeviation_PositiveForVariableData(t *testing.T) {
	metrics := makeLinearMetrics(10, 20, 10)
	sd := sla.CalculateStandardDeviation(metrics)
	if sd <= 0 {
		t.Errorf("expected positive stddev for variable series, got %f", sd)
	}
}

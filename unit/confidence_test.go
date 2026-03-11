package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/recommendation"
)

// makeMetricsSummary builds a MetricsSummary with sane defaults.
// Adjust specific fields in each test to exercise the scoring logic.
func makeMetricsSummary(samples int, durationHours float64, cv float64) recommendation.MetricsSummary {
	now := time.Now()
	oldest := now.Add(-time.Duration(durationHours * float64(time.Hour)))
	return recommendation.MetricsSummary{
		SampleCount:      samples,
		OldestSample:     oldest,
		NewestSample:     now,
		Mean:             500,
		StdDev:           500 * cv, // StdDev/Mean = CV
		Min:              100,
		Max:              900,
		ExpectedInterval: time.Minute,
		TimeGaps:         nil,
	}
}

// TestConfidenceCalculator_ScoreInRange verifies that CalculateConfidence
// always returns an overall score in [0, 100].
func TestConfidenceCalculator_ScoreInRange(t *testing.T) {
	calc := recommendation.NewConfidenceCalculator()

	cases := []struct {
		name    string
		samples int
		hours   float64
		cv      float64
	}{
		{"minimal data", 5, 0.1, 2.0},
		{"good data", 500, 24, 0.2},
		{"ideal data", 1000, 168, 0.1},
		{"high variance", 100, 6, 1.5},
	}

	for _, tc := range cases {
		s := makeMetricsSummary(tc.samples, tc.hours, tc.cv)
		score := calc.CalculateConfidence(s)
		if score.Score < 0 || score.Score > 100 {
			t.Errorf("case %q: score %f is outside [0, 100]", tc.name, score.Score)
		}
	}
}

// TestConfidenceCalculator_MoreDataHigherScore verifies that a series with
// more samples and longer history receives a higher confidence than one
// with minimal data (all else being equal).
func TestConfidenceCalculator_MoreDataHigherScore(t *testing.T) {
	calc := recommendation.NewConfidenceCalculator()

	low := calc.CalculateConfidence(makeMetricsSummary(10, 1, 0.3))
	high := calc.CalculateConfidence(makeMetricsSummary(1000, 168, 0.1))

	if high.Score <= low.Score {
		t.Errorf("expected higher confidence for more data (got low=%.1f, high=%.1f)",
			low.Score, high.Score)
	}
}

// TestConfidenceCalculator_LevelsAreKnown verifies that every confidence
// level string is one of the documented values.
func TestConfidenceCalculator_LevelsAreKnown(t *testing.T) {
	known := map[string]bool{
		"VeryLow": true, "Low": true, "Medium": true, "High": true, "VeryHigh": true,
	}
	calc := recommendation.NewConfidenceCalculator()

	cases := []recommendation.MetricsSummary{
		makeMetricsSummary(5, 0.1, 2.0),
		makeMetricsSummary(500, 24, 0.2),
		makeMetricsSummary(1000, 168, 0.05),
	}

	for _, s := range cases {
		score := calc.CalculateConfidence(s)
		lvl := string(score.Level)
		if !known[lvl] {
			t.Errorf("unknown confidence level %q", lvl)
		}
	}
}

// TestConfidenceCalculator_FromSamples_ScoreInRange verifies that
// CalculateFromSamples also returns a score in [0, 100].
func TestConfidenceCalculator_FromSamples_ScoreInRange(t *testing.T) {
	calc := recommendation.NewConfidenceCalculator()

	n := 100
	ts := make([]time.Time, n)
	vs := make([]int64, n)
	for i := range ts {
		ts[i] = time.Now().Add(-time.Duration(n-i) * time.Minute)
		vs[i] = int64(400 + i*2)
	}

	score := calc.CalculateFromSamples(ts, vs, time.Minute)
	if score.Score < 0 || score.Score > 100 {
		t.Errorf("CalculateFromSamples score %f is outside [0, 100]", score.Score)
	}
}

// TestConfidenceCalculator_FormatBreakdownNotEmpty verifies that
// FormatScoreBreakdown returns a non-empty descriptive string.
func TestConfidenceCalculator_FormatBreakdownNotEmpty(t *testing.T) {
	calc := recommendation.NewConfidenceCalculator()
	score := calc.CalculateConfidence(makeMetricsSummary(200, 12, 0.2))
	out := score.FormatScoreBreakdown()
	if out == "" {
		t.Error("FormatScoreBreakdown should not be empty")
	}
}

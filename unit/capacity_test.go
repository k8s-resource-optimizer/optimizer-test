package unit_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/trends"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// genLinearFromZero returns n values: 0, step, 2*step, ...
func genLinearFromZero(n int, step float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = float64(i) * step
	}
	return data
}

// makeTrendReport builds a minimal TrendReport with one workload entry
// for use in export tests.
func makeTrendReport() *trends.TrendReport {
	return &trends.TrendReport{
		GeneratedAt:    time.Now(),
		Namespace:      "test-ns",
		LookbackPeriod: 24 * time.Hour,
		WorkloadTrends: []trends.WorkloadTrend{
			{
				Namespace: "test-ns",
				Workload:  "my-app",
				CPUAnalysis: trends.TrendAnalysis{
					GrowthPattern: trends.PatternLinear,
					GrowthRate:    trends.GrowthRate{Daily: 1.0, Weekly: 7.0, Monthly: 30.0},
					CapacityStatus: trends.CapacityStatus{
						CurrentUtilization: 45.0,
						RiskLevel:          trends.RiskLow,
						RiskScore:          20.0,
						RecommendedAction:  trends.ActionMonitor,
						Message:            "Looks fine",
					},
					Confidence:  75.0,
					DataQuality: "good",
					DataPoints:  200,
				},
				MemoryAnalysis: trends.TrendAnalysis{
					GrowthPattern: trends.PatternStable,
					GrowthRate:    trends.GrowthRate{Daily: 0.1, Weekly: 0.7, Monthly: 3.0},
					CapacityStatus: trends.CapacityStatus{
						CurrentUtilization: 30.0,
						RiskLevel:          trends.RiskNone,
						RiskScore:          5.0,
						RecommendedAction:  trends.ActionMonitor,
						Message:            "Stable",
					},
					Confidence:  80.0,
					DataQuality: "good",
					DataPoints:  200,
				},
			},
		},
		TotalWorkloads: 1,
	}
}

// ─── DefaultAnalyzerConfig tests ────────────────────────────────────────────

// TestDefaultAnalyzerConfig_HasPositiveValues verifies that the default
// configuration has positive values for all thresholds and horizons.
func TestDefaultAnalyzerConfig_HasPositiveValues(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()

	if cfg.MinDataPoints <= 0 {
		t.Errorf("MinDataPoints should be positive, got %d", cfg.MinDataPoints)
	}
	if cfg.LookbackDefault <= 0 {
		t.Errorf("LookbackDefault should be positive, got %v", cfg.LookbackDefault)
	}
	if cfg.StableThreshold <= 0 {
		t.Errorf("StableThreshold should be positive, got %f", cfg.StableThreshold)
	}
	if cfg.SoftLimitPercent <= 0 || cfg.SoftLimitPercent >= 100 {
		t.Errorf("SoftLimitPercent should be in (0,100), got %f", cfg.SoftLimitPercent)
	}
	if cfg.HardLimitPercent <= cfg.SoftLimitPercent {
		t.Errorf("HardLimitPercent (%.1f) should exceed SoftLimitPercent (%.1f)",
			cfg.HardLimitPercent, cfg.SoftLimitPercent)
	}
	if cfg.ShortTermDays <= 0 {
		t.Errorf("ShortTermDays should be positive, got %d", cfg.ShortTermDays)
	}
	if cfg.MidTermDays <= cfg.ShortTermDays {
		t.Errorf("MidTermDays (%d) should exceed ShortTermDays (%d)", cfg.MidTermDays, cfg.ShortTermDays)
	}
	if cfg.LongTermDays <= cfg.MidTermDays {
		t.Errorf("LongTermDays (%d) should exceed MidTermDays (%d)", cfg.LongTermDays, cfg.MidTermDays)
	}
}

// ─── CalculateAcceleration tests ────────────────────────────────────────────

// TestCalculateAcceleration_ShortSliceReturnsZero verifies graceful handling
// of series with fewer than 3 points (returns 0).
func TestCalculateAcceleration_ShortSliceReturnsZero(t *testing.T) {
	cases := [][]float64{
		{},
		{1.0},
		{1.0, 2.0},
	}
	for _, data := range cases {
		acc := trends.CalculateAcceleration(data)
		if acc != 0 {
			t.Errorf("expected 0 acceleration for %d-element slice, got %f", len(data), acc)
		}
	}
}

// ─── CalculateCAGR tests ─────────────────────────────────────────────────────

// TestCalculateCAGR_NegativeForShrinkingData verifies that a shrinking series
// produces a negative CAGR.
func TestCalculateCAGR_NegativeForShrinkingData(t *testing.T) {
	data := genExponential(100, 100, 0.98) // slowly decreasing
	cagr := trends.CalculateCAGR(data, len(data))
	if cagr >= 0 {
		t.Errorf("expected negative CAGR for shrinking series, got %f", cagr)
	}
}

// TestCalculateCAGR_ZeroForEdgeCases verifies that edge cases (empty slice,
// single element, zero values) return 0 without panicking.
func TestCalculateCAGR_ZeroForEdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		data    []float64
		periods int
	}{
		{"empty", []float64{}, 0},
		{"single", []float64{100}, 1},
		{"zero start", []float64{0, 100, 200}, 3},
		{"zero end", []float64{100, 50, 0}, 3},
		{"zero periods", []float64{100, 200}, 0},
	}

	for _, tc := range cases {
		cagr := trends.CalculateCAGR(tc.data, tc.periods)
		if cagr != 0 {
			t.Errorf("case %q: expected CAGR=0 for edge case, got %f", tc.name, cagr)
		}
	}
}

// ─── CalculateRiskScore tests ────────────────────────────────────────────────

// TestCalculateRiskScore_HighUtilizationHighRisk verifies that near-100%
// utilization produces a high risk score (≥40 from utilization factor alone).
func TestCalculateRiskScore_HighUtilizationHighRisk(t *testing.T) {
	// 95% utilization → 95 * 0.4 = 38 points from utilization alone
	score := trends.CalculateRiskScore(95.0, nil, 0, nil)
	if score < 30 {
		t.Errorf("expected risk score ≥30 for 95%% utilization, got %f", score)
	}
}

// TestCalculateRiskScore_LowUtilizationLowRisk verifies that 10% utilization
// produces a low risk score (≤10 from utilization factor alone).
func TestCalculateRiskScore_LowUtilizationLowRisk(t *testing.T) {
	score := trends.CalculateRiskScore(10.0, nil, 0, nil)
	if score > 10 {
		t.Errorf("expected risk score ≤10 for 10%% utilization, got %f", score)
	}
}

// TestCalculateRiskScore_NearTermLimitIncreasesRisk verifies that a short
// time-to-limit (1 day) adds significant risk compared to no limit.
func TestCalculateRiskScore_NearTermLimitIncreasesRisk(t *testing.T) {
	baseScore := trends.CalculateRiskScore(50.0, nil, 0, nil)

	oneDayAhead := 24 * time.Hour
	scoreWithLimit := trends.CalculateRiskScore(50.0, &oneDayAhead, 0, nil)

	if scoreWithLimit <= baseScore {
		t.Errorf("1-day time-to-limit should increase risk score: base=%f, withLimit=%f",
			baseScore, scoreWithLimit)
	}
}

// TestCalculateRiskScore_IsInRange verifies the score is always in [0, 100].
func TestCalculateRiskScore_IsInRange(t *testing.T) {
	cases := []struct {
		util  float64
		acc   float64
		ttl   *time.Duration
	}{
		{0, 0, nil},
		{100, 5, nil},
		{50, 2, durationPtr(24 * time.Hour)},
		{90, 3, durationPtr(7 * 24 * time.Hour)},
	}

	for _, tc := range cases {
		score := trends.CalculateRiskScore(tc.util, tc.ttl, tc.acc, nil)
		if score < 0 || score > 100 {
			t.Errorf("risk score %.1f is outside [0,100] for util=%.0f acc=%.1f",
				score, tc.util, tc.acc)
		}
	}
}

func durationPtr(d time.Duration) *time.Duration { return &d }

// ─── RecommendCapacityAction tests ──────────────────────────────────────────

// TestRecommendCapacityAction_CriticalIsImmediate verifies that critical risk
// always maps to ActionImmediate regardless of time-to-limit.
func TestRecommendCapacityAction_CriticalIsImmediate(t *testing.T) {
	action := trends.RecommendCapacityAction(trends.RiskCritical, nil)
	if action != trends.ActionImmediate {
		t.Errorf("expected ActionImmediate for RiskCritical, got %s", action)
	}

	longTTL := 90 * 24 * time.Hour
	action = trends.RecommendCapacityAction(trends.RiskCritical, &longTTL)
	if action != trends.ActionImmediate {
		t.Errorf("expected ActionImmediate for RiskCritical even with long TTL, got %s", action)
	}
}

// TestRecommendCapacityAction_HighWithNearTermIsImmediate verifies that high
// risk with a short time-to-limit (< 7 days) maps to ActionImmediate.
func TestRecommendCapacityAction_HighWithNearTermIsImmediate(t *testing.T) {
	shortTTL := 3 * 24 * time.Hour // 3 days
	action := trends.RecommendCapacityAction(trends.RiskHigh, &shortTTL)
	if action != trends.ActionImmediate {
		t.Errorf("expected ActionImmediate for RiskHigh with 3-day TTL, got %s", action)
	}
}

// TestRecommendCapacityAction_HighWithLongTermIsPlan verifies that high risk
// with a distant time-to-limit maps to ActionPlan (not immediate).
func TestRecommendCapacityAction_HighWithLongTermIsPlan(t *testing.T) {
	longTTL := 60 * 24 * time.Hour // 60 days
	action := trends.RecommendCapacityAction(trends.RiskHigh, &longTTL)
	if action != trends.ActionPlan {
		t.Errorf("expected ActionPlan for RiskHigh with 60-day TTL, got %s", action)
	}
}

// TestRecommendCapacityAction_LowAndNoneIsMonitor verifies that low and no
// risk levels map to ActionMonitor.
func TestRecommendCapacityAction_LowAndNoneIsMonitor(t *testing.T) {
	for _, level := range []trends.RiskLevel{trends.RiskLow, trends.RiskNone} {
		action := trends.RecommendCapacityAction(level, nil)
		if action != trends.ActionMonitor {
			t.Errorf("expected ActionMonitor for %s risk, got %s", level, action)
		}
	}
}

// ─── PredictTimeToExhaustion tests ──────────────────────────────────────────

// TestPredictTimeToExhaustion_ZeroDataReturnsNone verifies that empty data
// returns a RiskNone status without panicking.
func TestPredictTimeToExhaustion_ZeroDataReturnsNone(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	status := trends.PredictTimeToExhaustion([]float64{}, nil, 100, cfg)
	if status == nil {
		t.Fatal("expected non-nil status for empty data")
	}
	if status.RiskLevel != trends.RiskNone {
		t.Errorf("expected RiskNone for empty data, got %s", status.RiskLevel)
	}
}

// TestPredictTimeToExhaustion_OverLimitIsCritical verifies that current
// usage already at or above the limit returns RiskCritical immediately.
func TestPredictTimeToExhaustion_OverLimitIsCritical(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	// 200 usage against 100 limit = 200% utilization
	data := make([]float64, 10)
	for i := range data {
		data[i] = 200.0
	}
	ts := genTimestamps(10, time.Hour)

	status := trends.PredictTimeToExhaustion(data, ts, 100, cfg)
	if status.RiskLevel != trends.RiskCritical {
		t.Errorf("expected RiskCritical when usage exceeds limit, got %s", status.RiskLevel)
	}
}

// TestPredictTimeToExhaustion_LowUsageLowRisk verifies that data at 10% of
// the limit returns a low-to-none risk status.
func TestPredictTimeToExhaustion_LowUsageLowRisk(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	// 10 units usage against 1000 limit = 1% utilization
	data := make([]float64, 50)
	for i := range data {
		data[i] = 10.0
	}
	ts := genTimestamps(50, time.Hour)

	status := trends.PredictTimeToExhaustion(data, ts, 1000, cfg)
	if status.RiskLevel == trends.RiskCritical || status.RiskLevel == trends.RiskHigh {
		t.Errorf("expected low/none risk for 1%% utilization, got %s", status.RiskLevel)
	}
}

// TestPredictTimeToExhaustion_ZeroLimitReturnsNone verifies that an invalid
// (zero) limit is handled gracefully.
func TestPredictTimeToExhaustion_ZeroLimitReturnsNone(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	data := genLinear(20, 100, 1)
	ts := genTimestamps(20, time.Hour)

	status := trends.PredictTimeToExhaustion(data, ts, 0, cfg)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.RiskLevel != trends.RiskNone {
		t.Errorf("expected RiskNone for zero limit, got %s", status.RiskLevel)
	}
}

// TestPredictTimeToExhaustion_MessageNotEmpty verifies that every returned
// status has a non-empty message.
func TestPredictTimeToExhaustion_MessageNotEmpty(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	data := genLinear(50, 10, 1)
	ts := genTimestamps(50, time.Hour)

	status := trends.PredictTimeToExhaustion(data, ts, 200, cfg)
	if status.Message == "" {
		t.Error("expected non-empty Message in CapacityStatus")
	}
}

// ─── TrendReport export tests ────────────────────────────────────────────────

// TestTrendReport_ExportJSON_ValidJSON verifies that ExportJSON produces
// valid, non-empty JSON containing the namespace name.
func TestTrendReport_ExportJSON_ValidJSON(t *testing.T) {
	report := makeTrendReport()
	var buf bytes.Buffer
	if err := report.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON returned error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("ExportJSON produced empty output")
	}
	if !strings.Contains(out, "test-ns") {
		t.Errorf("ExportJSON output missing namespace 'test-ns':\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		preview := out
		if len(preview) > 50 {
			preview = preview[:50]
		}
		t.Errorf("ExportJSON output does not start with '{': %s", preview)
	}
}

// TestTrendReport_ExportCSV_HasHeader verifies that ExportCSV produces CSV
// output with a header row containing expected column names.
func TestTrendReport_ExportCSV_HasHeader(t *testing.T) {
	report := makeTrendReport()
	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV returned error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("ExportCSV produced empty output")
	}

	expectedHeaders := []string{"Namespace", "Workload", "Resource", "Growth Pattern"}
	for _, h := range expectedHeaders {
		if !strings.Contains(out, h) {
			t.Errorf("ExportCSV output missing expected header column %q", h)
		}
	}
}

// TestTrendReport_ExportCSV_ContainsWorkloadData verifies that ExportCSV
// includes the workload name and resource types in data rows.
func TestTrendReport_ExportCSV_ContainsWorkloadData(t *testing.T) {
	report := makeTrendReport()
	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV returned error: %v", err)
	}
	out := buf.String()

	for _, expected := range []string{"my-app", "CPU", "Memory"} {
		if !strings.Contains(out, expected) {
			t.Errorf("ExportCSV output missing %q", expected)
		}
	}
}

// TestTrendReport_ExportHTML_IsValidHTML verifies that ExportHTML produces
// non-empty output containing basic HTML structure and the namespace.
func TestTrendReport_ExportHTML_IsValidHTML(t *testing.T) {
	report := makeTrendReport()
	var buf bytes.Buffer
	if err := report.ExportHTML(&buf); err != nil {
		t.Fatalf("ExportHTML returned error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("ExportHTML produced empty output")
	}

	for _, marker := range []string{"<!DOCTYPE html>", "<html", "</html>", "test-ns"} {
		if !strings.Contains(out, marker) {
			t.Errorf("ExportHTML output missing expected marker %q", marker)
		}
	}
}

// TestTrendReport_ExportJSON_EmptyWorkloads verifies that a report with no
// workloads can still be exported without error.
func TestTrendReport_ExportJSON_EmptyWorkloads(t *testing.T) {
	report := &trends.TrendReport{
		GeneratedAt: time.Now(),
		Namespace:   "empty-ns",
	}
	var buf bytes.Buffer
	if err := report.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON on empty report returned error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("ExportJSON should produce non-empty output even for empty report")
	}
}


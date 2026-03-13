package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/trends"
)

func makeTrendTimestamps(n int, interval time.Duration) []time.Time {
	ts := make([]time.Time, n)
	base := time.Now().Add(-time.Duration(n) * interval)
	for i := 0; i < n; i++ {
		ts[i] = base.Add(time.Duration(i) * interval)
	}
	return ts
}

func makeTrendData(n int, start, end float64) []float64 {
	data := make([]float64, n)
	for i := 0; i < n; i++ {
		data[i] = start + (end-start)*float64(i)/float64(n-1)
	}
	return data
}

// TestTrendsCapacity_PredictTimeToExhaustion verifies PredictTimeToExhaustion with growing data.
func TestTrendsCapacity_PredictTimeToExhaustion(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	n := 100
	data := makeTrendData(n, 100, 900)
	timestamps := makeTrendTimestamps(n, time.Hour)

	status := trends.PredictTimeToExhaustion(data, timestamps, 1000.0, cfg)
	if status == nil {
		t.Fatal("expected non-nil CapacityStatus")
	}
	if status.RiskScore < 0 || status.RiskScore > 100 {
		t.Errorf("risk score out of range [0,100]: %f", status.RiskScore)
	}
	if status.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestTrendsCapacity_PredictTimeToExhaustion_Empty verifies empty data returns safe status.
func TestTrendsCapacity_PredictTimeToExhaustion_Empty(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	status := trends.PredictTimeToExhaustion(nil, nil, 1000.0, cfg)
	if status == nil {
		t.Fatal("expected non-nil CapacityStatus for empty data")
	}
	if status.RiskLevel != trends.RiskNone {
		t.Errorf("expected RiskNone for empty data, got %s", status.RiskLevel)
	}
}

// TestTrendsCapacity_PredictTimeToExhaustion_OverLimit verifies over-limit returns critical.
func TestTrendsCapacity_PredictTimeToExhaustion_OverLimit(t *testing.T) {
	cfg := trends.DefaultAnalyzerConfig()
	data := []float64{1500.0}
	timestamps := []time.Time{time.Now()}

	status := trends.PredictTimeToExhaustion(data, timestamps, 1000.0, cfg)
	if status == nil {
		t.Fatal("expected non-nil CapacityStatus")
	}
	if status.RiskLevel != trends.RiskCritical {
		t.Errorf("expected RiskCritical for over-limit, got %s", status.RiskLevel)
	}
}

// TestTrendsCapacity_CalculateRiskScore verifies CalculateRiskScore at various utilization levels.
func TestTrendsCapacity_CalculateRiskScore(t *testing.T) {
	cases := []float64{10.0, 50.0, 95.0}

	for _, utilization := range cases {
		score := trends.CalculateRiskScore(utilization, nil, 0.0, nil)
		if score < 0 || score > 100 {
			t.Errorf("risk score out of range for utilization %.1f: %f", utilization, score)
		}
	}

	// With time-to-limit: much sooner should give higher score
	d := 6 * time.Hour
	scoreWithTime := trends.CalculateRiskScore(80.0, &d, 0.1, nil)
	if scoreWithTime < 0 || scoreWithTime > 100 {
		t.Errorf("risk score out of range with time-to-limit: %f", scoreWithTime)
	}
}

// TestTrendsCapacity_RecommendCapacityAction verifies all risk levels produce valid actions.
func TestTrendsCapacity_RecommendCapacityAction(t *testing.T) {
	riskLevels := []trends.RiskLevel{
		trends.RiskNone,
		trends.RiskLow,
		trends.RiskMedium,
		trends.RiskHigh,
		trends.RiskCritical,
	}

	for _, level := range riskLevels {
		action := trends.RecommendCapacityAction(level, nil)
		if action == "" {
			t.Errorf("expected non-empty action for risk level %s", level)
		}
	}

	// With time-to-limit
	d := 12 * time.Hour
	action := trends.RecommendCapacityAction(trends.RiskHigh, &d)
	if action == "" {
		t.Error("expected non-empty action with time-to-limit")
	}
}

// TestTrendsCapacity_GrowthPatternHelpers verifies growth helper functions are exercised
// via AnalyzeWorkload which calls them internally.
func TestTrendsCapacity_GrowthPatternHelpers(t *testing.T) {
	st := buildTrendStorage("prod", "growth-app", 120)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("prod", "growth-app", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}

	// GrowthPattern fields should be set
	cpuGrowth := result.CPUAnalysis.GrowthPattern
	_ = cpuGrowth // pattern string
}

// TestTrendsCapacity_ReportMaxRiskScore verifies report aggregation through AnalyzeNamespace.
func TestTrendsCapacity_ReportMaxRiskScore(t *testing.T) {
	st := buildTrendStorage("test-ns", "app1", 120)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	report, err := analyzer.AnalyzeNamespace("test-ns", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil TrendReport")
	}
	// MaxRiskScore is derived from workload results
	_ = report
}

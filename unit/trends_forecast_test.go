package unit_test

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
	"intelligent-cluster-optimizer/pkg/trends"
)

// buildHourlyTrendStorage creates a storage with n hourly PodMetric entries
// that have a clear 24-hour seasonal pattern.  The timestamps are spaced
// exactly 1 hour apart so that the Holt-Winters model can detect and fit the
// daily seasonality, which makes generateForecasts and createForecast reachable.
func buildHourlyTrendStorage(namespace, podPrefix string, n int) *storage.InMemoryStorage {
	s := storage.NewStorage()
	podName := podPrefix + "-abc123-xyz"
	base := time.Now().Add(-time.Duration(n) * time.Hour)

	for i := 0; i < n; i++ {
		phase := 2 * math.Pi * float64(i) / 24.0 // daily cycle
		cpu := int64(300 + 100*math.Sin(phase) + float64(i)*0.2)
		mem := int64(256*1024*1024 + int64(50*1024*1024)*int64(math.Sin(phase)*1000)/1000 + int64(i)*1024)
		if cpu < 50 {
			cpu = 50
		}
		if mem < 64*1024*1024 {
			mem = 64 * 1024 * 1024
		}
		s.Add(models.PodMetric{
			PodName:   podName,
			Namespace: namespace,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      cpu,
					UsageMemory:   mem,
					LimitCPU:      4000,
					LimitMemory:   int64(1024 * 1024 * 1024),
				},
			},
		})
	}
	return s
}

// ─── generateForecasts / createForecast coverage ────────────────────────────

// TestTrendAnalyzer_AnalyzeWorkload_ForecastProjectionsGenerated verifies that
// when AnalyzeWorkload succeeds with enough seasonal data, the short-term,
// mid-term and long-term forecast projections have their Horizon field set.
func TestTrendAnalyzer_AnalyzeWorkload_ForecastProjectionsGenerated(t *testing.T) {
	// 100 hourly data points: more than 2× the 24-period seasonal requirement.
	s := buildHourlyTrendStorage("prod", "web", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	result, err := analyzer.AnalyzeWorkload("prod", "web", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}

	// Even if fitting failed inside (empty forecast), the Horizon is always set.
	if result.CPUAnalysis.ShortTerm.Horizon != cfg.ShortTermDays {
		t.Errorf("expected ShortTerm.Horizon=%d, got %d",
			cfg.ShortTermDays, result.CPUAnalysis.ShortTerm.Horizon)
	}
	if result.CPUAnalysis.MidTerm.Horizon != cfg.MidTermDays {
		t.Errorf("expected MidTerm.Horizon=%d, got %d",
			cfg.MidTermDays, result.CPUAnalysis.MidTerm.Horizon)
	}
	if result.CPUAnalysis.LongTerm.Horizon != cfg.LongTermDays {
		t.Errorf("expected LongTerm.Horizon=%d, got %d",
			cfg.LongTermDays, result.CPUAnalysis.LongTerm.Horizon)
	}
}

// TestTrendAnalyzer_AnalyzeWorkload_DataQualityNotEmpty verifies that the
// data-quality string returned by AnalyzeWorkload is non-empty.
func TestTrendAnalyzer_AnalyzeWorkload_DataQualityNotEmpty(t *testing.T) {
	s := buildHourlyTrendStorage("staging", "api", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	result, err := analyzer.AnalyzeWorkload("staging", "api", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result.CPUAnalysis.DataQuality == "" {
		t.Error("expected non-empty DataQuality for CPU analysis")
	}
	if result.MemoryAnalysis.DataQuality == "" {
		t.Error("expected non-empty DataQuality for memory analysis")
	}
}

// ─── TrendReport export tests ────────────────────────────────────────────────

// buildSampleReport builds a TrendReport with two WorkloadTrends so that
// ExportCSV, ExportHTML, prepareTemplateData, and maxRiskScore are all exercised
// (sort.Slice only calls the comparator with 2+ elements).
func buildSampleReport() *trends.TrendReport {
	wt1 := trends.WorkloadTrend{
		Namespace: "test-ns",
		Workload:  "my-app",
		CPUAnalysis: trends.TrendAnalysis{
			GrowthPattern: trends.PatternLinear,
			Confidence:    80.0,
			DataQuality:   "good",
			CapacityStatus: trends.CapacityStatus{
				RiskScore:          55.0,
				RiskLevel:          trends.RiskMedium,
				RecommendedAction:  trends.ActionMonitor,
				CurrentUtilization: 65.0,
			},
		},
		MemoryAnalysis: trends.TrendAnalysis{
			GrowthPattern: trends.PatternLinear,
			Confidence:    75.0,
			DataQuality:   "good",
			CapacityStatus: trends.CapacityStatus{
				RiskScore:          70.0,
				RiskLevel:          trends.RiskHigh,
				RecommendedAction:  trends.ActionPlan,
				CurrentUtilization: 78.0,
			},
		},
		AnalysisTime: time.Now(),
	}
	// Second workload with lower risk to force sort comparison.
	wt2 := trends.WorkloadTrend{
		Namespace: "test-ns",
		Workload:  "sidecar",
		CPUAnalysis: trends.TrendAnalysis{
			GrowthPattern: trends.PatternStable,
			Confidence:    90.0,
			DataQuality:   "excellent",
			CapacityStatus: trends.CapacityStatus{
				RiskScore:          10.0,
				RiskLevel:          trends.RiskLow,
				RecommendedAction:  trends.ActionMonitor,
				CurrentUtilization: 20.0,
			},
		},
		MemoryAnalysis: trends.TrendAnalysis{
			GrowthPattern: trends.PatternStable,
			Confidence:    88.0,
			DataQuality:   "excellent",
			CapacityStatus: trends.CapacityStatus{
				RiskScore:          5.0,
				RiskLevel:          trends.RiskNone,
				RecommendedAction:  trends.ActionMonitor,
				CurrentUtilization: 10.0,
			},
		},
		AnalysisTime: time.Now(),
	}
	return &trends.TrendReport{
		GeneratedAt:    time.Now(),
		Namespace:      "test-ns",
		WorkloadTrends: []trends.WorkloadTrend{wt1, wt2},
		TotalWorkloads: 2,
	}
}

// TestTrendReport_ExportCSV_ContainsWorkload verifies that ExportCSV produces
// output that contains the workload name.
func TestTrendReport_ExportCSV_ContainsWorkload(t *testing.T) {
	report := buildSampleReport()
	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "my-app") {
		t.Errorf("expected CSV output to contain workload name 'my-app', got:\n%s", out)
	}
}

// TestTrendReport_ExportCSV_HasNamespaceHeader verifies that the CSV output starts with
// the expected header row (covers formatTimeToLimit nil branch via capacity_test helper).
func TestTrendReport_ExportCSV_HasNamespaceHeader(t *testing.T) {
	report := buildSampleReport()
	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "Namespace") {
		t.Error("expected CSV header row to contain 'Namespace'")
	}
}

// TestTrendReport_ExportCSV_TimeToLimitNil verifies that a nil TimeToLimit
// value in a CapacityPrediction is formatted as "N/A" in the CSV.
func TestTrendReport_ExportCSV_TimeToLimitNil(t *testing.T) {
	report := buildSampleReport()
	// TimeToLimit is already nil by default in CapacityPrediction.
	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "N/A") {
		t.Error("expected 'N/A' in CSV for nil TimeToLimit")
	}
}

// TestTrendReport_ExportCSV_TimeToLimitHours verifies that a sub-day
// TimeToLimit is formatted with "hours".
func TestTrendReport_ExportCSV_TimeToLimitHours(t *testing.T) {
	report := buildSampleReport()
	ttl := 6 * time.Hour
	report.WorkloadTrends[0].CPUAnalysis.CapacityStatus.TimeToLimit = &ttl

	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "hours") {
		t.Errorf("expected 'hours' in CSV for 6h TimeToLimit, got:\n%s", buf.String())
	}
}

// TestTrendReport_ExportCSV_TimeToLimitDays verifies that a time-to-limit
// between 1 day and 1 week is formatted with "days".
func TestTrendReport_ExportCSV_TimeToLimitDays(t *testing.T) {
	report := buildSampleReport()
	ttl := 3 * 24 * time.Hour // 3 days
	report.WorkloadTrends[0].CPUAnalysis.CapacityStatus.TimeToLimit = &ttl

	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "days") {
		t.Errorf("expected 'days' in CSV for 3-day TimeToLimit, got:\n%s", buf.String())
	}
}

// TestTrendReport_ExportCSV_TimeToLimitWeeks verifies that a time-to-limit
// between 1 week and 1 month is formatted with "weeks".
func TestTrendReport_ExportCSV_TimeToLimitWeeks(t *testing.T) {
	report := buildSampleReport()
	ttl := 14 * 24 * time.Hour // 14 days = 2 weeks
	report.WorkloadTrends[0].CPUAnalysis.CapacityStatus.TimeToLimit = &ttl

	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "weeks") {
		t.Errorf("expected 'weeks' in CSV for 14-day TimeToLimit, got:\n%s", buf.String())
	}
}

// TestTrendReport_ExportCSV_TimeToLimitMonths verifies that a time-to-limit
// over 30 days is formatted with "months".
func TestTrendReport_ExportCSV_TimeToLimitMonths(t *testing.T) {
	report := buildSampleReport()
	ttl := 60 * 24 * time.Hour // 60 days = 2 months
	report.WorkloadTrends[0].CPUAnalysis.CapacityStatus.TimeToLimit = &ttl

	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "months") {
		t.Errorf("expected 'months' in CSV for 60-day TimeToLimit, got:\n%s", buf.String())
	}
}

// TestTrendReport_ExportHTML_NonEmpty verifies that ExportHTML produces a
// non-empty HTML string containing basic HTML structure.
func TestTrendReport_ExportHTML_NonEmpty(t *testing.T) {
	report := buildSampleReport()
	var buf bytes.Buffer
	if err := report.ExportHTML(&buf); err != nil {
		t.Fatalf("ExportHTML error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<html") {
		t.Error("expected ExportHTML output to contain '<html'")
	}
}

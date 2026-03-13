package integration_test

import (
	"bytes"
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/trends"
)


// TestTrendsReportPipeline_ExportCSV verifies the full
// storage → TrendAnalyzer → TrendReport → ExportCSV pipeline.
func TestTrendsReportPipeline_ExportCSV(t *testing.T) {
	st := buildTrendStorage("prod", "worker", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	report, err := analyzer.AnalyzeNamespace("prod", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil TrendReport")
	}

	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Namespace") {
		t.Error("expected CSV to contain 'Namespace' header")
	}
}

// TestTrendsReportPipeline_ExportHTML verifies the ExportHTML pipeline.
func TestTrendsReportPipeline_ExportHTML(t *testing.T) {
	st := buildTrendStorage("prod", "api", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	report, err := analyzer.AnalyzeNamespace("prod", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace error: %v", err)
	}

	var buf bytes.Buffer
	if err := report.ExportHTML(&buf); err != nil {
		t.Fatalf("ExportHTML error: %v", err)
	}
	if !strings.Contains(buf.String(), "<html") {
		t.Error("expected ExportHTML to contain '<html'")
	}
}

// TestTrendsReportPipeline_MultipleNamespaces verifies that analyzing two different
// namespaces produces independent reports.
func TestTrendsReportPipeline_MultipleNamespaces(t *testing.T) {
	st1 := buildTrendStorage("ns-a", "svc", 100)
	st2 := buildTrendStorage("ns-b", "svc", 100)

	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48

	report1, err := trends.NewTrendAnalyzer(st1, cfg).AnalyzeNamespace("ns-a", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace ns-a error: %v", err)
	}
	report2, err := trends.NewTrendAnalyzer(st2, cfg).AnalyzeNamespace("ns-b", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace ns-b error: %v", err)
	}

	if report1.Namespace == report2.Namespace {
		t.Error("expected different namespaces in independent reports")
	}
}

// TestTrendsReportPipeline_CapacityRiskScore verifies the capacity risk score
// calculation is exercised via AnalyzeWorkload → CapacityStatus.
func TestTrendsReportPipeline_CapacityRiskScore(t *testing.T) {
	st := buildTrendStorage("prod", "risky", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("prod", "risky", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}

	riskScore := result.CPUAnalysis.CapacityStatus.RiskScore
	if riskScore < 0 || riskScore > 100 {
		t.Errorf("expected risk score in [0,100], got %.1f", riskScore)
	}
}

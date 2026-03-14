package integration_test

import (
	"bytes"
	"math"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
	"intelligent-cluster-optimizer/pkg/trends"
)

// buildExponentialStorage creates storage with exponentially growing CPU data.
// This triggers testExponentialFit path in DetectGrowthPattern.
func buildExponentialStorage(namespace, podPrefix string, n int) *storage.InMemoryStorage {
	st := storage.NewStorage()
	podName := podPrefix + "-abc-xyz"
	base := time.Now().Add(-time.Duration(n) * time.Hour)

	for i := 0; i < n; i++ {
		// Strong exponential growth: doubles every ~100 steps
		cpu := int64(100 * math.Exp(float64(i)*0.015))
		if cpu > 8000 {
			cpu = 8000
		}
		mem := int64(128 * 1024 * 1024)
		st.Add(models.PodMetric{
			PodName:   podName,
			Namespace: namespace,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      cpu,
					UsageMemory:   mem,
					LimitCPU:      16000,
					LimitMemory:   int64(4 * 1024 * 1024 * 1024),
				},
			},
		})
	}
	return st
}

// buildLogarithmicStorage creates storage with logarithmically growing data.
func buildLogarithmicStorage(namespace, podPrefix string, n int) *storage.InMemoryStorage {
	st := storage.NewStorage()
	podName := podPrefix + "-abc-xyz"
	base := time.Now().Add(-time.Duration(n) * time.Hour)

	for i := 0; i < n; i++ {
		// Logarithmic growth: fast at start, slows down
		cpu := int64(100 + 500*math.Log1p(float64(i)))
		mem := int64(128 * 1024 * 1024)
		st.Add(models.PodMetric{
			PodName:   podName,
			Namespace: namespace,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      cpu,
					UsageMemory:   mem,
					LimitCPU:      16000,
					LimitMemory:   int64(4 * 1024 * 1024 * 1024),
				},
			},
		})
	}
	return st
}

// buildMultiWorkloadTrendStorage creates storage with multiple workloads.
func buildMultiWorkloadTrendStorage(namespace string, workloads []string, n int) *storage.InMemoryStorage {
	st := storage.NewStorage()
	base := time.Now().Add(-time.Duration(n) * time.Hour)

	for wi, workload := range workloads {
		podName := workload + "-abc-xyz"
		for i := 0; i < n; i++ {
			phase := 2 * 3.14159 * float64(i) / 24.0
			sinVal := phase - phase*phase*phase/6 + phase*phase*phase*phase*phase/120
			cpu := int64(200+float64(50*(wi+1))*sinVal) + int64(float64(i)*float64(wi+1)*0.3)
			if cpu < 50 {
				cpu = 50
			}
			mem := int64(128 * 1024 * 1024)
			st.Add(models.PodMetric{
				PodName:   podName,
				Namespace: namespace,
				Timestamp: base.Add(time.Duration(i) * time.Hour),
				Containers: []models.ContainerMetric{
					{
						ContainerName: "app",
						UsageCPU:      cpu,
						UsageMemory:   mem,
						LimitCPU:      4000,
						LimitMemory:   int64(2 * 1024 * 1024 * 1024),
					},
				},
			})
		}
	}
	return st
}

// TestTrendsExtended2_ExponentialGrowthPattern exercises testExponentialFit via
// DetectGrowthPattern on exponential data.
func TestTrendsExtended2_ExponentialGrowthPattern(t *testing.T) {
	st := buildExponentialStorage("exp-ns", "exp-app", 120)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("exp-ns", "exp-app", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}
	_ = result.CPUAnalysis.GrowthPattern
}

// TestTrendsExtended2_LogarithmicGrowthPattern exercises testLogarithmicFit path.
func TestTrendsExtended2_LogarithmicGrowthPattern(t *testing.T) {
	st := buildLogarithmicStorage("log-ns", "log-app", 120)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("log-ns", "log-app", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}
	_ = result.CPUAnalysis.GrowthPattern
}

// TestTrendsExtended2_MultiWorkloadExportHTML verifies ExportHTML with multiple workloads,
// triggering maxRiskScore comparison during sort.
func TestTrendsExtended2_MultiWorkloadExportHTML(t *testing.T) {
	workloads := []string{"frontend", "backend", "worker"}
	st := buildMultiWorkloadTrendStorage("multi-ns", workloads, 120)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	report, err := analyzer.AnalyzeNamespace("multi-ns", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil TrendReport")
	}

	// ExportHTML with 2+ workloads triggers maxRiskScore sort comparison
	var buf bytes.Buffer
	if err := report.ExportHTML(&buf); err != nil {
		t.Fatalf("ExportHTML error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty HTML output")
	}
}

// TestTrendsExtended2_ExportCSVWithTimeToLimit verifies formatTimeToLimit non-nil path.
func TestTrendsExtended2_ExportCSVWithTimeToLimit(t *testing.T) {
	// Use exponential data so capacity forecasting is triggered
	st := buildExponentialStorage("caplimit-ns", "fast-grow", 120)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	report, err := analyzer.AnalyzeNamespace("caplimit-ns", cfg.LookbackDefault)
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
}

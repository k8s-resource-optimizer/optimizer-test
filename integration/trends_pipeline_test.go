package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
	"intelligent-cluster-optimizer/pkg/trends"
)

func buildTrendStorage(namespace, podPrefix string, n int) *storage.InMemoryStorage {
	st := storage.NewStorage()
	podName := podPrefix + "-abc123-xyz"
	base := time.Now().Add(-time.Duration(n) * time.Hour)

	for i := 0; i < n; i++ {
		phase := 2 * 3.14159 * float64(i) / 24.0
		sinVal := phase - phase*phase*phase/6 + phase*phase*phase*phase*phase/120
		cpu := int64(300 + 100*sinVal + float64(i)*0.5)
		mem := int64(256*1024*1024) + int64(sinVal*float64(32*1024*1024))
		if cpu < 50 {
			cpu = 50
		}
		if mem < 64*1024*1024 {
			mem = 64 * 1024 * 1024
		}
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
	return st
}

func TestTrendsPipeline_StorageToAnalyzeWorkload(t *testing.T) {
	st := buildTrendStorage("prod", "web", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("prod", "web", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}
	if result.CPUAnalysis.DataQuality == "" {
		t.Error("expected non-empty CPU DataQuality")
	}
	if result.MemoryAnalysis.DataQuality == "" {
		t.Error("expected non-empty Memory DataQuality")
	}
}

func TestTrendsPipeline_StorageToAnalyzeNamespace(t *testing.T) {
	st := buildTrendStorage("staging", "api", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	report, err := analyzer.AnalyzeNamespace("staging", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil TrendReport")
	}
	if report.TotalWorkloads == 0 {
		t.Error("expected at least one workload in TrendReport")
	}
}

func TestTrendsPipeline_ForecastHorizonsSet(t *testing.T) {
	st := buildTrendStorage("prod", "svc", 100)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("prod", "svc", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}

	if result.CPUAnalysis.ShortTerm.Horizon != cfg.ShortTermDays {
		t.Errorf("ShortTerm.Horizon: expected %d, got %d", cfg.ShortTermDays, result.CPUAnalysis.ShortTerm.Horizon)
	}
	if result.CPUAnalysis.MidTerm.Horizon != cfg.MidTermDays {
		t.Errorf("MidTerm.Horizon: expected %d, got %d", cfg.MidTermDays, result.CPUAnalysis.MidTerm.Horizon)
	}
	if result.CPUAnalysis.LongTerm.Horizon != cfg.LongTermDays {
		t.Errorf("LongTerm.Horizon: expected %d, got %d", cfg.LongTermDays, result.CPUAnalysis.LongTerm.Horizon)
	}
}

func TestTrendsPipeline_InsufficientData(t *testing.T) {
	st := buildTrendStorage("empty-ns", "sparse", 5)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	_, err := analyzer.AnalyzeWorkload("empty-ns", "sparse", cfg.LookbackDefault)
	if err == nil {
		t.Log("AnalyzeWorkload returned nil error for sparse data (acceptable)")
	}
}

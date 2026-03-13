package unit_test

import (
	"fmt"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
	"intelligent-cluster-optimizer/pkg/trends"
)

// buildTrendStorage creates an InMemoryStorage populated with n PodMetric
// entries for a single pod whose name follows the pattern
// "<podPrefix>-abc123-xyz" so extractWorkloadName trims the last two
// hyphen-separated segments and returns podPrefix as the workload name.
// Each metric has one container with CPU usage varying between ~200-400m and
// memory usage between ~200-400 MiB. Timestamps are spaced 1 minute apart
// going back n minutes from now.
func buildTrendStorage(namespace, podPrefix string, n int) *storage.InMemoryStorage {
	s := storage.NewStorage()
	podName := fmt.Sprintf("%s-abc123-xyz", podPrefix)

	for i := 0; i < n; i++ {
		// Vary CPU between 200 and 400 millicores using a simple oscillation.
		cpu := int64(300 + 100*(i%3-1))
		// Vary memory between 200 and 400 MiB.
		mem := int64((200 + (i%3)*100) * 1024 * 1024)

		s.Add(models.PodMetric{
			PodName:   podName,
			Namespace: namespace,
			Timestamp: time.Now().Add(-time.Duration(n-i) * time.Minute),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      cpu,
					UsageMemory:   mem,
				},
			},
		})
	}

	return s
}

// TestTrendAnalyzer_NewWithNilConfig verifies that passing nil config uses
// defaults and does not panic.
func TestTrendAnalyzer_NewWithNilConfig(t *testing.T) {
	s := storage.NewStorage()
	analyzer := trends.NewTrendAnalyzer(s, nil)
	if analyzer == nil {
		t.Fatal("expected non-nil TrendAnalyzer")
	}
}

// TestTrendAnalyzer_NewWithNilConfig_NoDefaultsNil verifies that a nil config
// still produces a usable analyzer (no panic on subsequent call).
func TestTrendAnalyzer_NewWithNilConfig_NoDefaultsNil(t *testing.T) {
	s := storage.NewStorage()
	// Should not panic.
	_ = trends.NewTrendAnalyzer(s, nil)
}

// TestTrendAnalyzer_AnalyzeWorkload_InsufficientData verifies that
// AnalyzeWorkload returns an error when fewer than MinDataPoints metrics exist.
// DefaultAnalyzerConfig.MinDataPoints = 48; we supply only 5 entries.
func TestTrendAnalyzer_AnalyzeWorkload_InsufficientData(t *testing.T) {
	s := buildTrendStorage("default", "my-app", 5)
	analyzer := trends.NewTrendAnalyzer(s, nil)

	_, err := analyzer.AnalyzeWorkload("default", "my-app", time.Hour)
	if err == nil {
		t.Error("expected error for insufficient data, got nil")
	}
}

// TestTrendAnalyzer_AnalyzeWorkload_SufficientData verifies that
// AnalyzeWorkload returns a non-nil WorkloadTrend when enough data is present.
// We use a custom config with MinDataPoints=10 and a matching storage size.
func TestTrendAnalyzer_AnalyzeWorkload_SufficientData(t *testing.T) {
	const n = 15
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 10
	cfg.LookbackDefault = time.Duration(n+5) * time.Minute

	s := buildTrendStorage("prod", "web-server", n)
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	result, err := analyzer.AnalyzeWorkload("prod", "web-server", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}
}

// TestTrendAnalyzer_AnalyzeWorkload_NamespaceAndWorkloadSet verifies that the
// returned WorkloadTrend has the correct Namespace and Workload fields.
func TestTrendAnalyzer_AnalyzeWorkload_NamespaceAndWorkloadSet(t *testing.T) {
	const n = 15
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 10
	cfg.LookbackDefault = time.Duration(n+5) * time.Minute

	s := buildTrendStorage("staging", "api-service", n)
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	result, err := analyzer.AnalyzeWorkload("staging", "api-service", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload: %v", err)
	}
	if result.Namespace != "staging" {
		t.Errorf("expected Namespace=%q, got %q", "staging", result.Namespace)
	}
	if result.Workload != "api-service" {
		t.Errorf("expected Workload=%q, got %q", "api-service", result.Workload)
	}
}

// TestTrendAnalyzer_AnalyzeWorkload_AnalysisTimeNonZero verifies that
// AnalysisTime is set to a non-zero value.
func TestTrendAnalyzer_AnalyzeWorkload_AnalysisTimeNonZero(t *testing.T) {
	const n = 15
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 10
	cfg.LookbackDefault = time.Duration(n+5) * time.Minute

	s := buildTrendStorage("ns1", "worker", n)
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	result, err := analyzer.AnalyzeWorkload("ns1", "worker", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload: %v", err)
	}
	if result.AnalysisTime.IsZero() {
		t.Error("expected non-zero AnalysisTime")
	}
}

// TestTrendAnalyzer_AnalyzeNamespace_EmptyNamespaceError verifies that
// AnalyzeNamespace returns an error for a namespace with no metrics.
func TestTrendAnalyzer_AnalyzeNamespace_EmptyNamespaceError(t *testing.T) {
	s := storage.NewStorage()
	analyzer := trends.NewTrendAnalyzer(s, nil)

	_, err := analyzer.AnalyzeNamespace("nonexistent", time.Hour)
	if err == nil {
		t.Error("expected error for namespace with no metrics, got nil")
	}
}

// TestTrendAnalyzer_AnalyzeNamespace_ReturnsTrendReport verifies that
// AnalyzeNamespace returns a non-nil TrendReport with at least one workload
// when the namespace contains sufficient data.
func TestTrendAnalyzer_AnalyzeNamespace_ReturnsTrendReport(t *testing.T) {
	const n = 15
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 10
	cfg.LookbackDefault = time.Duration(n+5) * time.Minute

	s := buildTrendStorage("mynamespace", "frontend", n)
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	report, err := analyzer.AnalyzeNamespace("mynamespace", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil TrendReport")
	}
}

// TestTrendAnalyzer_AnalyzeNamespace_TotalWorkloadsPositive verifies that
// TotalWorkloads is greater than zero in the returned report.
func TestTrendAnalyzer_AnalyzeNamespace_TotalWorkloadsPositive(t *testing.T) {
	const n = 15
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 10
	cfg.LookbackDefault = time.Duration(n+5) * time.Minute

	s := buildTrendStorage("testns", "backend", n)
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	report, err := analyzer.AnalyzeNamespace("testns", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace: %v", err)
	}
	if report.TotalWorkloads <= 0 {
		t.Errorf("expected TotalWorkloads > 0, got %d", report.TotalWorkloads)
	}
}

// TestTrendAnalyzer_AnalyzeNamespace_NamespaceMatches verifies that the
// TrendReport Namespace field matches the input namespace.
func TestTrendAnalyzer_AnalyzeNamespace_NamespaceMatches(t *testing.T) {
	const n = 15
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 10
	cfg.LookbackDefault = time.Duration(n+5) * time.Minute

	const ns = "production"
	s := buildTrendStorage(ns, "svc", n)
	analyzer := trends.NewTrendAnalyzer(s, cfg)

	report, err := analyzer.AnalyzeNamespace(ns, cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeNamespace: %v", err)
	}
	if report.Namespace != ns {
		t.Errorf("expected Namespace=%q, got %q", ns, report.Namespace)
	}
}

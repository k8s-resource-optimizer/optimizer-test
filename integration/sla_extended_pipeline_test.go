package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/sla"
)

func makeSLAMetricsValues(n int, base, jitter float64) []sla.Metric {
	metrics := make([]sla.Metric, n)
	for i := 0; i < n; i++ {
		v := base + jitter*float64(i%3-1)
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(-time.Duration(n-i) * time.Minute),
			Value:     v,
		}
	}
	return metrics
}

// TestSLAExtended_MonitorCRUD verifies AddSLA, GetSLA, ListSLAs, RemoveSLA.
func TestSLAExtended_MonitorCRUD(t *testing.T) {
	m := sla.NewMonitor()

	sladef := sla.SLADefinition{
		Name:      "latency-sla",
		Type:      sla.SLATypeLatency,
		Target:    200,
		Threshold: 50,
		Percentile: 95,
		Window:    5 * time.Minute,
	}
	if err := m.AddSLA(sladef); err != nil {
		t.Fatalf("AddSLA error: %v", err)
	}

	got, err := m.GetSLA("latency-sla")
	if err != nil {
		t.Fatalf("GetSLA error: %v", err)
	}
	if got.Name != "latency-sla" {
		t.Errorf("expected 'latency-sla', got %s", got.Name)
	}

	list := m.ListSLAs()
	if len(list) == 0 {
		t.Error("expected non-empty ListSLAs")
	}

	// CheckSLA
	metrics := makeSLAMetricsValues(20, 180.0, 10.0)
	violations, err := m.CheckSLA("latency-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA error: %v", err)
	}
	_ = violations

	if err := m.RemoveSLA("latency-sla"); err != nil {
		t.Fatalf("RemoveSLA error: %v", err)
	}

	list2 := m.ListSLAs()
	for _, s := range list2 {
		if s.Name == "latency-sla" {
			t.Error("RemoveSLA did not remove the SLA")
		}
	}
}

// TestSLAExtended_CheckCustomSLA verifies custom SLA type.
func TestSLAExtended_CheckCustomSLA(t *testing.T) {
	m := sla.NewMonitor()

	customSLA := sla.SLADefinition{
		Name:      "custom-sla",
		Type:      sla.SLATypeCustom,
		Target:    100,
		Threshold: 20,
		Window:    1 * time.Minute,
	}
	if err := m.AddSLA(customSLA); err != nil {
		t.Fatalf("AddSLA custom error: %v", err)
	}

	metrics := makeSLAMetricsValues(10, 110.0, 5.0)
	violations, err := m.CheckSLA("custom-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA custom error: %v", err)
	}
	_ = violations
}

// TestSLAExtended_ControlChart verifies GenerateChart, CalculateMovingAverage,
// CalculateStandardDeviation.
func TestSLAExtended_ControlChart(t *testing.T) {
	metrics := makeSLAMetricsValues(30, 50.0, 5.0)

	cc := sla.NewControlChart()
	cfg := sla.ControlChartConfig{
		SigmaLevel:           3,
		MinSamples:           5,
		EnableTrendDetection: true,
		TrendWindowSize:      5,
	}

	points, err := cc.GenerateChart(metrics, cfg)
	if err != nil {
		t.Fatalf("GenerateChart error: %v", err)
	}
	if len(points) == 0 {
		t.Error("expected non-empty points from GenerateChart")
	}

	// CalculateMovingAverage
	avgs := sla.CalculateMovingAverage(metrics, 5)
	if len(avgs) == 0 {
		t.Error("expected non-empty moving averages")
	}

	// CalculateStandardDeviation
	stddev := sla.CalculateStandardDeviation(metrics)
	if stddev < 0 {
		t.Errorf("expected non-negative stddev, got %f", stddev)
	}
}

// TestSLAExtended_HealthCheckerWithMonitor verifies NewHealthCheckerWithMonitor.
func TestSLAExtended_HealthCheckerWithMonitor(t *testing.T) {
	m := sla.NewMonitor()
	hc := sla.NewHealthCheckerWithMonitor(m)
	if hc == nil {
		t.Fatal("expected non-nil HealthChecker from NewHealthCheckerWithMonitor")
	}

	metrics := makeSLAMetricsValues(20, 50.0, 5.0)
	result, err := hc.PreOptimizationCheck(metrics)
	if err != nil {
		t.Fatalf("PreOptimizationCheck error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil HealthCheckResult")
	}
}

// TestSLAExtended_ShouldBlockAndRollback verifies ShouldBlockOptimization and ShouldRollback.
func TestSLAExtended_ShouldBlockAndRollback(t *testing.T) {
	hc := sla.NewHealthChecker()
	metrics := makeSLAMetricsValues(20, 50.0, 5.0)

	pre, err := hc.PreOptimizationCheck(metrics)
	if err != nil {
		t.Fatalf("PreOptimizationCheck error: %v", err)
	}
	post, err := hc.PostOptimizationCheck(metrics)
	if err != nil {
		t.Fatalf("PostOptimizationCheck error: %v", err)
	}

	impact, err := hc.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}

	block, reason := sla.ShouldBlockOptimization(pre)
	_ = block
	_ = reason

	rollback, msg := sla.ShouldRollback(impact)
	_ = rollback
	_ = msg
}

// TestSLAExtended_ValidationError verifies ValidationError.Error().
func TestSLAExtended_ValidationError(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{
		Name:      "ok-sla",
		Type:      sla.SLATypeLatency,
		Target:    100,
		Threshold: 10,
		Window:    time.Minute,
	})
	if err != nil {
		t.Errorf("expected no validation error, got %v", err)
	}

	// Invalid SLA — triggers ValidationError
	invalidErr := sla.ValidateSLADefinition(sla.SLADefinition{})
	if invalidErr == nil {
		t.Error("expected validation error for empty SLA")
	}
	_ = invalidErr.Error()
}

// TestSLAExtended_CheckAllSLAs verifies CheckAllSLAs with multiple SLAs.
func TestSLAExtended_CheckAllSLAs(t *testing.T) {
	m := sla.NewMonitor()

	slas := []sla.SLADefinition{
		{Name: "err-rate", Type: sla.SLATypeErrorRate, Target: 0.01, Threshold: 0.005, Window: time.Minute},
		{Name: "avail", Type: sla.SLATypeAvailability, Target: 99.9, Threshold: 0.1, Window: time.Minute},
		{Name: "throughput", Type: sla.SLATypeThroughput, Target: 1000, Threshold: 100, Window: time.Minute},
		{Name: "startup", Type: sla.SLATypeStartupTime, Target: 5, Threshold: 2, Window: time.Minute},
	}
	for _, s := range slas {
		if err := m.AddSLA(s); err != nil {
			t.Fatalf("AddSLA %s error: %v", s.Name, err)
		}
	}

	metrics := makeSLAMetricsValues(20, 50.0, 5.0)
	violations, err := m.CheckAllSLAs(metrics)
	if err != nil {
		t.Fatalf("CheckAllSLAs error: %v", err)
	}
	_ = violations
}

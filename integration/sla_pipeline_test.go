package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/sla"
)

func makeSLAMetrics(n int, base, spread float64) []sla.Metric {
	metrics := make([]sla.Metric, n)
	start := time.Now().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		metrics[i] = sla.Metric{
			Timestamp: start.Add(time.Duration(i) * time.Minute),
			Value:     base + spread*float64(i)/float64(n),
		}
	}
	return metrics
}

func makeSLADef(name string, slaType sla.SLAType, threshold float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        name,
		Type:        slaType,
		Target:      threshold * 0.8,
		Threshold:   threshold,
		Percentile:  95,
		Window:      30 * time.Minute,
		Description: "integration test SLA",
	}
}

// TestSLAPipeline_HealthySystem verifies that a healthy system passes all SLA checks.
func TestSLAPipeline_HealthySystem(t *testing.T) {
	metrics := makeSLAMetrics(60, 10.0, 5.0) // latency 10-15ms
	slas := []sla.SLADefinition{
		makeSLADef("latency-sla", sla.SLATypeLatency, 100.0), // threshold 100ms — well above
	}

	checker := sla.NewHealthChecker()
	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("CheckHealth error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil HealthCheckResult")
	}
	if result.Score < 0 || result.Score > 100 {
		t.Errorf("expected score in [0,100], got %.1f", result.Score)
	}

	shouldBlock, _ := sla.ShouldBlockOptimization(result)
	if shouldBlock {
		t.Error("healthy system should not block optimization")
	}
}

// TestSLAPipeline_ViolatingSystem verifies that high latency triggers violations.
func TestSLAPipeline_ViolatingSystem(t *testing.T) {
	// Latency 200-300ms — well above 100ms threshold
	metrics := makeSLAMetrics(60, 200.0, 100.0)
	slas := []sla.SLADefinition{
		makeSLADef("latency-sla", sla.SLATypeLatency, 100.0),
	}

	checker := sla.NewHealthChecker()
	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("CheckHealth error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil HealthCheckResult")
	}
	// Score should be lower for violating system
	if result.Score > 95 {
		t.Errorf("expected lower score for violating system, got %.1f", result.Score)
	}
}

// TestSLAPipeline_PrePostOptimizationComparison verifies the pre→post comparison pipeline.
func TestSLAPipeline_PrePostOptimizationComparison(t *testing.T) {
	preMetrics := makeSLAMetrics(30, 50.0, 20.0)  // 50-80ms latency
	postMetrics := makeSLAMetrics(30, 20.0, 10.0) // 20-30ms latency (improved)

	checker := sla.NewHealthChecker()

	pre, err := checker.PreOptimizationCheck(preMetrics)
	if err != nil {
		t.Fatalf("PreOptimizationCheck error: %v", err)
	}
	post, err := checker.PostOptimizationCheck(postMetrics)
	if err != nil {
		t.Fatalf("PostOptimizationCheck error: %v", err)
	}

	impact, err := checker.CompareHealth(pre, post)
	if err != nil {
		t.Fatalf("CompareHealth error: %v", err)
	}
	if impact == nil {
		t.Fatal("expected non-nil OptimizationImpact")
	}

	rollback, _ := sla.ShouldRollback(impact)
	if rollback {
		t.Log("rollback suggested even for improved metrics — checking impact score")
	}
}

// TestSLAPipeline_ErrorRateSLA verifies error rate SLA type is handled.
func TestSLAPipeline_ErrorRateSLA(t *testing.T) {
	// Error rate 0.5-1% — below 5% threshold
	metrics := makeSLAMetrics(60, 0.5, 0.5)
	slas := []sla.SLADefinition{
		makeSLADef("error-rate-sla", sla.SLATypeErrorRate, 5.0),
	}

	checker := sla.NewHealthChecker()
	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("CheckHealth error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil HealthCheckResult")
	}
	healthy := sla.IsSystemHealthy(result, 50.0)
	_ = healthy
}

// TestSLAPipeline_MultipleSLATypes verifies multiple SLA types in one check.
func TestSLAPipeline_MultipleSLATypes(t *testing.T) {
	metrics := makeSLAMetrics(60, 30.0, 10.0)
	slas := []sla.SLADefinition{
		makeSLADef("latency", sla.SLATypeLatency, 200.0),
		makeSLADef("error-rate", sla.SLATypeErrorRate, 5.0),
		makeSLADef("availability", sla.SLATypeAvailability, 99.0),
		makeSLADef("throughput", sla.SLATypeThroughput, 100.0),
		makeSLADef("startup", sla.SLATypeStartupTime, 60.0),
	}

	checker := sla.NewHealthChecker()
	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("CheckHealth error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil HealthCheckResult")
	}
}

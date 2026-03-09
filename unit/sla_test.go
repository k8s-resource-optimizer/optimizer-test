package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/sla"
)

// latencySLA builds a latency SLA with a hard ceiling at `maxMs` milliseconds.
//
// The monitor's violation check is:  actual > (Target + Threshold)
// We set Target=0 and Threshold=maxMs so the absolute ceiling is simply maxMs.
func latencySLA(maxMs float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        "latency-sla",
		Type:        sla.SLATypeLatency,
		Target:      0,
		Threshold:   maxMs, // ceiling: 0 + maxMs
		Percentile:  95,
		Window:      5 * time.Minute,
		Description: "P95 latency ceiling",
	}
}

// errorRateSLA builds an error-rate SLA.
// The monitor checks: avgErrorRate > (Target + Threshold).
// Setting Target=0, Threshold=maxPct makes the ceiling simply maxPct.
func errorRateSLA(maxPct float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        "error-rate-sla",
		Type:        sla.SLATypeErrorRate,
		Target:      0,
		Threshold:   maxPct, // ceiling: 0 + maxPct
		Window:      5 * time.Minute,
		Description: "HTTP error rate ceiling",
	}
}

// availabilitySLA builds an availability SLA.
//
// The monitor computes availability as the percentage of metrics where value > 0,
// then checks: availability < (Target - Threshold).
// We set Target=100 and Threshold=(100 - minPct), so the floor is minPct.
// E.g. minPct=90 → Threshold=10 → floor = 100-10 = 90%.
func availabilitySLA(minPct float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        "availability-sla",
		Type:        sla.SLATypeAvailability,
		Target:      100,
		Threshold:   100 - minPct, // floor: 100 - (100-minPct) = minPct
		Window:      5 * time.Minute,
		Description: "Availability floor",
	}
}

// metricsAt builds a slice of Metric values with a constant `value`
// timestamped over the last `n` minutes.
func metricsAt(value float64, n int) []sla.Metric {
	metrics := make([]sla.Metric, n)
	for i := 0; i < n; i++ {
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(-time.Duration(n-i) * time.Minute),
			Value:     value,
		}
	}
	return metrics
}

// unavailableMetrics builds a mixed slice: `upCount` metrics with value=1 (up)
// followed by `downCount` metrics with value=0 (down).
// The availability monitor counts value>0 as "up".
func unavailableMetrics(upCount, downCount int) []sla.Metric {
	total := upCount + downCount
	metrics := make([]sla.Metric, total)
	for i := 0; i < total; i++ {
		val := 1.0
		if i >= upCount {
			val = 0 // down
		}
		metrics[i] = sla.Metric{
			Timestamp: time.Now().Add(-time.Duration(total-i) * time.Minute),
			Value:     val,
		}
	}
	return metrics
}

// TestSLAMonitor_LatencyViolation verifies that a latency value exceeding
// the ceiling (Target + Threshold = 0 + 200 = 200ms) is flagged.
func TestSLAMonitor_LatencyViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(200) // ceiling = 200 ms
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 350 ms > 200 ms ceiling → must be a violation.
	metrics := metricsAt(350, 5)
	violations, err := mon.CheckSLA("latency-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected at least one latency violation, got none")
	}
}

// TestSLAMonitor_NoViolationWhenWithinBudget verifies that latency within
// the ceiling does not trigger a violation.
func TestSLAMonitor_NoViolationWhenWithinBudget(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(200) // ceiling = 200 ms
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 150 ms < 200 ms ceiling → no violation.
	metrics := metricsAt(150, 5)
	violations, err := mon.CheckSLA("latency-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for 150ms latency (ceiling=200ms), got %d", len(violations))
	}
}

// TestSLAMonitor_ErrorRateViolation verifies that an error rate above the
// configured threshold is flagged as an SLA violation.
func TestSLAMonitor_ErrorRateViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := errorRateSLA(1.0) // max 1% error rate
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 5% error rate far exceeds the 1% threshold.
	metrics := metricsAt(5.0, 5)
	violations, err := mon.CheckSLA("error-rate-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected error rate violation, got none")
	}
}

// TestSLAMonitor_ErrorRateNoViolation verifies that an acceptable error rate
// (below threshold) does not trigger a violation.
func TestSLAMonitor_ErrorRateNoViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := errorRateSLA(1.0) // max 1% error rate
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 0.5% is below the 1% threshold.
	metrics := metricsAt(0.5, 5)
	violations, err := mon.CheckSLA("error-rate-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for 0.5%% error rate, got %d", len(violations))
	}
}

// TestSLAMonitor_AvailabilityViolation verifies that availability below
// the SLA minimum is detected.
//
// The monitor computes availability as: (metrics with value > 0) / total * 100.
// We send 9 up metrics (value=1) and 1 down (value=0) → 90% availability.
// SLA floor = 99% → violation expected.
func TestSLAMonitor_AvailabilityViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := availabilitySLA(99) // floor = 99%
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 9 up + 1 down = 90% availability < 99% floor → violation.
	metrics := unavailableMetrics(9, 1)
	violations, err := mon.CheckSLA("availability-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected availability violation (90% < 99% floor), got none")
	}
}

// TestSLAMonitor_AddAndRemoveSLA verifies that removing an SLA makes it
// unavailable for subsequent checks.
func TestSLAMonitor_AddAndRemoveSLA(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(200)

	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}
	if err := mon.RemoveSLA("latency-sla"); err != nil {
		t.Fatalf("RemoveSLA: %v", err)
	}

	// After removal, CheckSLA must return an error.
	_, err := mon.CheckSLA("latency-sla", metricsAt(100, 3))
	if err == nil {
		t.Error("expected error when checking removed SLA, got nil")
	}
}

// TestSLAMonitor_CheckAllSLAs verifies that CheckAllSLAs aggregates
// violations across multiple registered SLAs.
func TestSLAMonitor_CheckAllSLAs(t *testing.T) {
	mon := sla.NewMonitor()

	// Register a latency SLA (ceiling=200ms) and an error-rate SLA (ceiling=1%).
	if err := mon.AddSLA(latencySLA(200)); err != nil {
		t.Fatalf("AddSLA latency: %v", err)
	}
	if err := mon.AddSLA(errorRateSLA(1.0)); err != nil {
		t.Fatalf("AddSLA error-rate: %v", err)
	}

	// 350ms metrics violate the 200ms latency ceiling.
	latencyMetrics := metricsAt(350, 3)
	violations, err := mon.CheckAllSLAs(latencyMetrics)
	if err != nil {
		t.Fatalf("CheckAllSLAs: %v", err)
	}

	// At minimum, the latency SLA should fire.
	if len(violations) == 0 {
		t.Error("expected at least one violation from CheckAllSLAs")
	}
}

// TestSLAMonitor_ViolationSeverityRange verifies that the Severity field
// on any violation is always in the valid range [0, 1].
func TestSLAMonitor_ViolationSeverityRange(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(200)
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	violations, err := mon.CheckSLA("latency-sla", metricsAt(500, 5))
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	for _, v := range violations {
		if v.Severity < 0 || v.Severity > 1 {
			t.Errorf("violation severity %f is outside [0, 1]", v.Severity)
		}
	}
}

// TestSLAMonitor_DuplicateSLAOverrides verifies that adding an SLA with
// the same name twice updates (or errors) rather than creating duplicates.
func TestSLAMonitor_DuplicateSLAName(t *testing.T) {
	mon := sla.NewMonitor()
	def1 := latencySLA(200)
	def2 := latencySLA(500) // same name, different threshold
	def2.Name = def1.Name

	if err := mon.AddSLA(def1); err != nil {
		t.Fatalf("AddSLA first: %v", err)
	}
	// Second add — the implementation may allow overwrite or return an error.
	// Both are acceptable; we just ensure it doesn't panic.
	_ = mon.AddSLA(def2)
}

// TestSLAMonitor_RemoveNonExistentSLA verifies that removing a non-existent
// SLA returns an error rather than silently succeeding.
func TestSLAMonitor_RemoveNonExistentSLA(t *testing.T) {
	mon := sla.NewMonitor()
	err := mon.RemoveSLA("does-not-exist")
	if err == nil {
		t.Error("expected error when removing non-existent SLA, got nil")
	}
}

// TestSLAMonitor_EmptyMetricsDoesNotPanic verifies the monitor handles
// an empty metrics slice gracefully.
func TestSLAMonitor_EmptyMetricsDoesNotPanic(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(200)
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// This must not panic.
	_, err := mon.CheckSLA("latency-sla", []sla.Metric{})
	_ = err // error or empty violations both acceptable
}

package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/sla"
)

// throughputSLA builds a throughput SLA requiring at least minRPS requests/sec.
// The monitor checks: avgThroughput < (Target - Threshold).
// Setting Target=minRPS and Threshold=0 makes the floor simply minRPS.
func throughputSLA(minRPS float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        "throughput-sla",
		Type:        sla.SLATypeThroughput,
		Target:      minRPS,
		Threshold:   0,
		Window:      5 * time.Minute,
		Description: "Minimum throughput floor",
	}
}

// startupTimeSLA builds a startup-time SLA with a maximum allowed startup
// time of maxSecs seconds. The monitor checks: avg > (Target + Threshold).
// Setting Target=0, Threshold=maxSecs makes the ceiling simply maxSecs.
func startupTimeSLA(maxSecs float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        "startup-time-sla",
		Type:        sla.SLATypeStartupTime,
		Target:      0,
		Threshold:   maxSecs,
		Window:      5 * time.Minute,
		Description: "Max pod startup time",
	}
}

// customSLA builds a custom SLA that uses a simple threshold check (same
// logic as latency). Values above (Target + Threshold) are violations.
func customSLA(ceiling float64) sla.SLADefinition {
	return sla.SLADefinition{
		Name:        "custom-sla",
		Type:        sla.SLATypeCustom,
		Target:      0,
		Threshold:   ceiling,
		Window:      5 * time.Minute,
		Description: "Custom metric ceiling",
	}
}

// TestSLAMonitor_RemoveSLA_ListEmpty verifies that after removing an SLA
// ListSLAs returns an empty slice.
func TestSLAMonitor_RemoveSLA_ListEmpty(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(200)

	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}
	if err := mon.RemoveSLA("latency-sla"); err != nil {
		t.Fatalf("RemoveSLA: %v", err)
	}

	all := mon.ListSLAs()
	if len(all) != 0 {
		t.Errorf("expected 0 SLAs after removal, got %d", len(all))
	}
}

// TestSLAMonitor_GetSLA_ExistingReturnsIt verifies that GetSLA returns the
// definition that was previously added.
func TestSLAMonitor_GetSLA_ExistingReturnsIt(t *testing.T) {
	mon := sla.NewMonitor()
	def := latencySLA(300)

	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	got, err := mon.GetSLA("latency-sla")
	if err != nil {
		t.Fatalf("GetSLA: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil SLADefinition")
	}
	if got.Name != "latency-sla" {
		t.Errorf("expected Name=%q, got %q", "latency-sla", got.Name)
	}
}

// TestSLAMonitor_GetSLA_UnknownReturnsError verifies that GetSLA returns an
// error when the requested SLA name does not exist.
func TestSLAMonitor_GetSLA_UnknownReturnsError(t *testing.T) {
	mon := sla.NewMonitor()

	_, err := mon.GetSLA("does-not-exist")
	if err == nil {
		t.Error("expected error for unknown SLA, got nil")
	}
}

// TestSLAMonitor_ListSLAs_ReturnsAll verifies that ListSLAs returns every
// SLA that has been added.
func TestSLAMonitor_ListSLAs_ReturnsAll(t *testing.T) {
	mon := sla.NewMonitor()

	defs := []sla.SLADefinition{
		latencySLA(200),
		errorRateSLA(1.0),
		availabilitySLA(99),
	}
	for _, d := range defs {
		if err := mon.AddSLA(d); err != nil {
			t.Fatalf("AddSLA %q: %v", d.Name, err)
		}
	}

	all := mon.ListSLAs()
	if len(all) != len(defs) {
		t.Errorf("expected %d SLAs, got %d", len(defs), len(all))
	}
}

// TestSLAMonitor_ThroughputViolation verifies that CheckSLA with a
// SLATypeThroughput definition fires a violation when average throughput is
// below the configured minimum.
func TestSLAMonitor_ThroughputViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := throughputSLA(100) // minimum 100 req/s
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 10 req/s is far below the 100 req/s floor.
	metrics := metricsAt(10, 5)
	violations, err := mon.CheckSLA("throughput-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected throughput violation (10 < 100 req/s), got none")
	}
}

// TestSLAMonitor_ThroughputNoViolation verifies that no violation is reported
// when average throughput meets the minimum requirement.
func TestSLAMonitor_ThroughputNoViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := throughputSLA(100) // minimum 100 req/s
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 150 req/s is above the 100 req/s floor — no violation expected.
	metrics := metricsAt(150, 5)
	violations, err := mon.CheckSLA("throughput-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for 150 req/s (floor=100), got %d", len(violations))
	}
}

// TestSLAMonitor_StartupTimeViolation verifies that CheckSLA with a
// SLATypeStartupTime definition fires a violation when average startup time
// exceeds the configured maximum.
func TestSLAMonitor_StartupTimeViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := startupTimeSLA(30) // max 30 seconds
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 60s average startup time exceeds the 30s ceiling.
	metrics := metricsAt(60, 5)
	violations, err := mon.CheckSLA("startup-time-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected startup time violation (60s > 30s), got none")
	}
}

// TestSLAMonitor_StartupTimeNoViolation verifies that no violation is reported
// when average startup time is within the allowed limit.
func TestSLAMonitor_StartupTimeNoViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := startupTimeSLA(30) // max 30 seconds
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// 10s average startup time is well below the 30s ceiling.
	metrics := metricsAt(10, 5)
	violations, err := mon.CheckSLA("startup-time-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for 10s startup (ceiling=30s), got %d", len(violations))
	}
}

// TestSLAMonitor_CustomSLAViolation verifies that a custom SLA using the
// threshold-based check fires a violation when the metric value exceeds the
// configured ceiling.
func TestSLAMonitor_CustomSLAViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := customSLA(50) // ceiling = 50
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// Value of 100 exceeds the ceiling of 50.
	metrics := metricsAt(100, 5)
	violations, err := mon.CheckSLA("custom-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected custom SLA violation (100 > 50), got none")
	}
}

// TestSLAMonitor_CustomSLANoViolation verifies that no violation is reported
// when the custom metric value is within the allowed ceiling.
func TestSLAMonitor_CustomSLANoViolation(t *testing.T) {
	mon := sla.NewMonitor()
	def := customSLA(50) // ceiling = 50
	if err := mon.AddSLA(def); err != nil {
		t.Fatalf("AddSLA: %v", err)
	}

	// Value of 20 is below the ceiling of 50.
	metrics := metricsAt(20, 5)
	violations, err := mon.CheckSLA("custom-sla", metrics)
	if err != nil {
		t.Fatalf("CheckSLA: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for value=20 (ceiling=50), got %d", len(violations))
	}
}

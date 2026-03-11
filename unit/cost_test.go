package unit_test

import (
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/cost"
)

// TestCalculateCost_PositiveValues verifies that a non-zero CPU and memory
// allocation produces positive hourly/daily/monthly costs.
func TestCalculateCost_PositiveValues(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("aws-us-east-1")
	rc := calc.CalculateCost(500, 256*1024*1024) // 0.5 cores, 256 MiB

	if rc.TotalPerHour <= 0 {
		t.Errorf("expected positive hourly cost, got %f", rc.TotalPerHour)
	}
	if rc.TotalPerDay <= 0 {
		t.Errorf("expected positive daily cost, got %f", rc.TotalPerDay)
	}
	if rc.TotalPerMonth <= 0 {
		t.Errorf("expected positive monthly cost, got %f", rc.TotalPerMonth)
	}
}

// TestCalculateCost_ZeroResources verifies that zero CPU and memory
// produce zero cost (no phantom charges).
func TestCalculateCost_ZeroResources(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")
	rc := calc.CalculateCost(0, 0)

	if rc.TotalPerHour != 0 || rc.TotalPerDay != 0 || rc.TotalPerMonth != 0 {
		t.Errorf("expected zero cost for zero resources, got hourly=%f", rc.TotalPerHour)
	}
}

// TestCalculateCost_YearlyApprox verifies that yearly ≈ monthly × 12.
func TestCalculateCost_YearlyApprox(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")
	rc := calc.CalculateCost(1000, 1024*1024*1024) // 1 core, 1 GiB

	expected := rc.TotalPerDay * 365
	diff := rc.TotalPerYear - expected
	if diff < 0 {
		diff = -diff
	}
	// Allow 1% tolerance due to floating point.
	if diff/expected > 0.01 {
		t.Errorf("yearly (%f) should be ≈ daily*365 (%f)", rc.TotalPerYear, expected)
	}
}

// TestEstimateSavings_PositiveSavingsWhenReduced verifies that reducing
// CPU and memory produces positive savings.
func TestEstimateSavings_PositiveSavingsWhenReduced(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("aws-us-east-1")
	est := calc.EstimateSavings(
		1000, 500, // CPU: current 1000m → recommended 500m
		512*1024*1024, 256*1024*1024, // Memory: 512 MiB → 256 MiB
	)

	if est.RecommendedCost.TotalPerHour >= est.CurrentCost.TotalPerHour {
		t.Errorf("expected recommended cost < current cost after reduction")
	}
}

// TestEstimateSavings_NegativeSavingsWhenIncreased verifies that increasing
// resource allocation results in a negative savings (i.e. higher cost).
func TestEstimateSavings_NegativeSavingsWhenIncreased(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")
	est := calc.EstimateSavings(
		500, 1000, // CPU increase
		256*1024*1024, 512*1024*1024, // Memory increase
	)

	if est.RecommendedCost.TotalPerHour <= est.CurrentCost.TotalPerHour {
		t.Errorf("expected recommended cost > current cost after increase")
	}
}

// TestEstimateWorkloadSavings_ScalesWithReplicas verifies that workload
// savings for 4 replicas are exactly 4× the per-container savings.
func TestEstimateWorkloadSavings_ScalesWithReplicas(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")

	containers := []cost.ContainerResourceChange{
		{
			ContainerName:   "app",
			CurrentCPU:      1000,
			RecommendedCPU:  600,
			CurrentMemory:   512 * 1024 * 1024,
			RecommendedMemory: 300 * 1024 * 1024,
		},
	}

	ws1 := calc.EstimateWorkloadSavings("default", "Deployment", "api", 1, containers)
	ws4 := calc.EstimateWorkloadSavings("default", "Deployment", "api", 4, containers)

	ratio := ws4.TotalSavings.CurrentCost.TotalPerMonth / ws1.TotalSavings.CurrentCost.TotalPerMonth
	// Allow 1% tolerance.
	if ratio < 3.96 || ratio > 4.04 {
		t.Errorf("4-replica workload cost should be 4× single-replica, got ratio %f", ratio)
	}
}

// TestGenerateReport_AggregatesWorkloads verifies that the report contains
// all workloads passed in and the total is positive.
func TestGenerateReport_AggregatesWorkloads(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")

	containers := []cost.ContainerResourceChange{
		{ContainerName: "app", CurrentCPU: 1000, RecommendedCPU: 500,
			CurrentMemory: 512 * 1024 * 1024, RecommendedMemory: 256 * 1024 * 1024},
	}

	workloads := []cost.WorkloadSavings{
		calc.EstimateWorkloadSavings("default", "Deployment", "api", 2, containers),
		calc.EstimateWorkloadSavings("default", "Deployment", "worker", 3, containers),
	}

	report := calc.GenerateReport(workloads)

	if len(report.Workloads) != 2 {
		t.Errorf("expected 2 workloads in report, got %d", len(report.Workloads))
	}
	if report.TotalSavings.CurrentCost.TotalPerMonth <= 0 {
		t.Error("expected positive total current cost in report")
	}
}

// TestFormatCost_ContainsKeywords verifies that FormatCost includes
// expected keywords for human-readable output.
func TestFormatCost_ContainsKeywords(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")
	rc := calc.CalculateCost(1000, 1*1024*1024*1024)
	out := rc.FormatCost()

	for _, kw := range []string{"/hour", "/day", "/month"} {
		if !strings.Contains(out, kw) {
			t.Errorf("FormatCost output missing keyword %q, got: %s", kw, out)
		}
	}
}

// TestFormatSavings_NotEmpty verifies that FormatSavings returns a non-empty string.
func TestFormatSavings_NotEmpty(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")
	est := calc.EstimateSavings(1000, 500, 512*1024*1024, 256*1024*1024)
	out := est.FormatSavings()
	if out == "" {
		t.Error("FormatSavings should return a non-empty string")
	}
}

// TestNewCalculator_NilPricingUsesDefault verifies that passing nil to
// NewCalculator falls back to a default pricing model (no panic, positive costs).
func TestNewCalculator_NilPricingUsesDefault(t *testing.T) {
	calc := cost.NewCalculator(nil)
	rc := calc.CalculateCost(1000, 1*1024*1024*1024)
	if rc.TotalPerHour <= 0 {
		t.Errorf("expected positive cost with nil pricing model (default fallback), got %f", rc.TotalPerHour)
	}
}

// TestPresets_AllReturnPositiveCost verifies that every named pricing preset
// produces positive costs (none are misconfigured with zero rates).
func TestPresets_AllReturnPositiveCost(t *testing.T) {
	presets := []string{"aws-us-east-1", "aws-us-east-1-spot", "gcp-us-central1", "azure-eastus", "default"}
	for _, p := range presets {
		calc := cost.NewCalculatorWithPreset(p)
		rc := calc.CalculateCost(1000, 1*1024*1024*1024)
		if rc.TotalPerHour <= 0 {
			t.Errorf("preset %q: expected positive hourly cost, got %f", p, rc.TotalPerHour)
		}
	}
}

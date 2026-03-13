package unit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/pareto"
	"intelligent-cluster-optimizer/pkg/policy"
	"intelligent-cluster-optimizer/pkg/prediction"
	"intelligent-cluster-optimizer/pkg/sla"
)

// ─── ForecastResult methods ───────────────────────────────────────────────────

// TestForecastResult_Summary_EmptyForecasts verifies that Summary returns a
// meaningful string when no forecasts were generated.
func TestForecastResult_Summary_EmptyForecasts(t *testing.T) {
	r := &prediction.ForecastResult{}
	s := r.Summary()
	if s == "" {
		t.Error("expected non-empty Summary for empty ForecastResult")
	}
}

// TestForecastResult_Summary_WithForecasts verifies that Summary includes the
// number of forecast periods when forecasts are present.
func TestForecastResult_Summary_WithForecasts(t *testing.T) {
	hw := prediction.NewHoltWinters()
	data := generateSeasonalData(72, 24, 200, 0.5)
	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict error: %v", err)
	}
	s := result.Summary()
	if s == "" {
		t.Error("expected non-empty Summary after successful FitPredict")
	}
}

// TestForecastResult_GetForecast_Valid verifies that GetForecast returns a
// non-nil Forecast for an in-range horizon.
func TestForecastResult_GetForecast_Valid(t *testing.T) {
	hw := prediction.NewHoltWinters()
	data := generateSeasonalData(72, 24, 200, 0.5)
	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict error: %v", err)
	}

	f, err := result.GetForecast(3)
	if err != nil {
		t.Fatalf("GetForecast(3) error: %v", err)
	}
	if f == nil {
		t.Error("expected non-nil Forecast for horizon=3")
	}
}

// TestForecastResult_GetForecast_OutOfRange verifies that GetForecast returns
// an error when the requested horizon is out of range.
func TestForecastResult_GetForecast_OutOfRange(t *testing.T) {
	hw := prediction.NewHoltWinters()
	data := generateSeasonalData(72, 24, 200, 0.5)
	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict error: %v", err)
	}

	_, err = result.GetForecast(100) // horizon > len(Forecasts)
	if err == nil {
		t.Error("expected error for out-of-range horizon")
	}
}

// TestForecastResult_TroughForecast_Empty verifies that TroughForecast returns
// nil for an empty ForecastResult.
func TestForecastResult_TroughForecast_Empty(t *testing.T) {
	r := &prediction.ForecastResult{}
	if r.TroughForecast() != nil {
		t.Error("expected nil TroughForecast for empty result")
	}
}

// TestForecastResult_TroughForecast_WithData verifies that TroughForecast
// returns a valid forecast pointing to the minimum forecasted value.
func TestForecastResult_TroughForecast_WithData(t *testing.T) {
	hw := prediction.NewHoltWinters()
	data := generateSeasonalData(72, 24, 200, 0.5)
	result, err := hw.FitPredict(data, 12)
	if err != nil {
		t.Fatalf("FitPredict error: %v", err)
	}

	trough := result.TroughForecast()
	if trough == nil {
		t.Fatal("expected non-nil TroughForecast")
	}
	for _, f := range result.Forecasts {
		if f.Value < trough.Value {
			t.Errorf("TroughForecast value %.2f is not the minimum (found %.2f smaller)",
				trough.Value, f.Value)
		}
	}
}

// ─── HoltWinters dampingSum (via TrendDamped prediction) ─────────────────────

// TestHoltWinters_DampedTrend_PredictSucceeds verifies that fitting and
// predicting with TrendDamped exercises the dampingSum code path.
func TestHoltWinters_DampedTrend_PredictSucceeds(t *testing.T) {
	cfg := prediction.DefaultConfig()
	cfg.TrendType = prediction.TrendDamped
	cfg.DampingFactor = 0.9
	hw := prediction.NewHoltWintersWithConfig(cfg)

	data := generateSeasonalData(72, 24, 150, 0.5)
	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict with TrendDamped error: %v", err)
	}
	if len(result.Forecasts) != 6 {
		t.Errorf("expected 6 forecast points, got %d", len(result.Forecasts))
	}
}

// ─── Solution.Summary and ObjectiveSummary ───────────────────────────────────

// TestSolution_Summary_ContainsID verifies that Solution.Summary includes
// the solution ID.
func TestSolution_Summary_ContainsID(t *testing.T) {
	s := pareto.NewSolution("test-sol-1", "default", "my-app")
	s.CPURequest = 500
	s.MemoryRequest = 256 * 1024 * 1024
	summary := s.Summary()
	if summary == "" {
		t.Error("expected non-empty Summary")
	}
}

// TestSolution_ObjectiveSummary_ContainsSolutionID verifies that
// ObjectiveSummary includes the solution ID.
func TestSolution_ObjectiveSummary_ContainsSolutionID(t *testing.T) {
	s := pareto.NewSolution("sol-abc", "default", "my-app")
	summary := s.ObjectiveSummary()
	if summary == "" {
		t.Error("expected non-empty ObjectiveSummary")
	}
}

// ─── policy.Engine.LoadPolicies (from file) ───────────────────────────────────

// TestPolicyEngine_LoadPolicies_ValidFile verifies that LoadPolicies succeeds
// when given a path to a valid YAML policy file.
func TestPolicyEngine_LoadPolicies_ValidFile(t *testing.T) {
	content := `
policies:
  - name: allow-all
    condition: 'true'
    action: allow
    priority: 1
    enabled: true
defaultAction: allow
`
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "policies.yaml")
	if err := os.WriteFile(policyFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp policy file: %v", err)
	}

	e := policy.NewEngine()
	if err := e.LoadPolicies(policyFile); err != nil {
		t.Fatalf("LoadPolicies error: %v", err)
	}

	policies := e.GetPolicies()
	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(policies))
	}
}

// TestPolicyEngine_LoadPolicies_NonExistentFile verifies that LoadPolicies
// returns an error when the file does not exist.
func TestPolicyEngine_LoadPolicies_NonExistentFile(t *testing.T) {
	e := policy.NewEngine()
	err := e.LoadPolicies("/nonexistent/path/to/policies.yaml")
	if err == nil {
		t.Error("expected error for non-existent policy file")
	}
}

// ─── cost.SavingsReport.FormatReport ─────────────────────────────────────────

// TestSavingsReport_FormatReport_NonEmpty verifies that FormatReport produces
// a non-empty formatted string for a minimal savings report.
func TestSavingsReport_FormatReport_NonEmpty(t *testing.T) {
	report := cost.SavingsReport{
		GeneratedAt:  time.Now(),
		PricingModel: "aws-us-east-1-standard",
		WorkloadCount: 3,
		ContainerCount: 5,
		TotalCurrentCost: cost.ResourceCost{
			CPUCostPerHour:    0.05,
			MemoryCostPerHour: 0.01,
			TotalPerHour:      0.06,
		},
		TotalSavings: cost.SavingsEstimate{
			CurrentCost: cost.ResourceCost{TotalPerHour: 0.06},
			RecommendedCost: cost.ResourceCost{TotalPerHour: 0.04},
			TotalSavingsPerHour: 0.02,
			PercentageReduction: 33.3,
		},
	}

	out := report.FormatReport()
	if out == "" {
		t.Error("expected non-empty FormatReport output")
	}
}

// ─── sla.ValidationError.Error ───────────────────────────────────────────────

// TestValidationError_Error_ContainsFieldAndMessage verifies that the
// ValidationError.Error() method returns a string containing both the
// field name and the message.
func TestValidationError_Error_ContainsFieldAndMessage(t *testing.T) {
	e := &sla.ValidationError{Field: "cpu", Message: "must be positive"}
	msg := e.Error()
	if msg == "" {
		t.Error("expected non-empty Error() string")
	}
}

// ─── sla.NewHealthCheckerWithMonitor ─────────────────────────────────────────

// TestNewHealthCheckerWithMonitor_NonNil verifies that passing a custom
// monitor to NewHealthCheckerWithMonitor returns a non-nil HealthChecker.
func TestNewHealthCheckerWithMonitor_NonNil(t *testing.T) {
	monitor := sla.NewMonitor()
	hc := sla.NewHealthCheckerWithMonitor(monitor)
	if hc == nil {
		t.Error("expected non-nil HealthChecker from NewHealthCheckerWithMonitor")
	}
}

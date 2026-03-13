package unit_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// ─── GetSeasonals tests ───────────────────────────────────────────────────────

// TestHoltWinters_GetSeasonals_BeforeFit verifies that GetSeasonals returns
// an empty (non-nil) slice when the model has not been fitted yet.
func TestHoltWinters_GetSeasonals_BeforeFit(t *testing.T) {
	hw := prediction.NewHoltWinters()
	s := hw.GetSeasonals()
	if s == nil {
		t.Error("expected non-nil slice from GetSeasonals before fitting")
	}
}

// TestHoltWinters_GetSeasonals_AfterFit verifies that after fitting, GetSeasonals
// returns a slice whose length matches the configured seasonal period.
func TestHoltWinters_GetSeasonals_AfterFit(t *testing.T) {
	const period = 24
	data := generateSeasonalData(3*period, period, 200, 0.5)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit error: %v", err)
	}

	s := hw.GetSeasonals()
	if len(s) != period {
		t.Errorf("expected %d seasonal components, got %d", period, len(s))
	}
}

// TestHoltWinters_GetSeasonals_IsCopy verifies that the slice returned by
// GetSeasonals is a copy — mutating it does not affect the model.
func TestHoltWinters_GetSeasonals_IsCopy(t *testing.T) {
	const period = 24
	data := generateSeasonalData(3*period, period, 100, 0)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit error: %v", err)
	}

	s1 := hw.GetSeasonals()
	if len(s1) == 0 {
		t.Skip("no seasonal components returned")
	}
	original := s1[0]
	s1[0] = original + 9999

	s2 := hw.GetSeasonals()
	if s2[0] != original {
		t.Error("mutating GetSeasonals result should not affect the model's internals")
	}
}

// ─── IsFitted tests ───────────────────────────────────────────────────────────

// TestHoltWinters_IsFitted_FalseBeforeFit verifies that a freshly created model
// reports itself as not fitted.
func TestHoltWinters_IsFitted_FalseBeforeFit(t *testing.T) {
	hw := prediction.NewHoltWinters()
	if hw.IsFitted() {
		t.Error("expected IsFitted()=false for a new, unfitted model")
	}
}

// TestHoltWinters_IsFitted_TrueAfterFit verifies that after a successful Fit
// call the model reports itself as fitted.
func TestHoltWinters_IsFitted_TrueAfterFit(t *testing.T) {
	const period = 24
	data := generateSeasonalData(3*period, period, 200, 0.5)
	hw := prediction.NewHoltWinters()
	if err := hw.Fit(data); err != nil {
		t.Fatalf("Fit error: %v", err)
	}
	if !hw.IsFitted() {
		t.Error("expected IsFitted()=true after successful Fit")
	}
}

// ─── FitPredict after custom config ──────────────────────────────────────────

// TestHoltWinters_FitPredict_DampedModel verifies that FitPredict works
// with a damped-trend model configuration.
func TestHoltWinters_FitPredict_DampedModel(t *testing.T) {
	cfg := prediction.DefaultConfig()
	cfg.TrendType = prediction.TrendAdditive
	cfg.DampingFactor = 0.95
	hw := prediction.NewHoltWintersWithConfig(cfg)

	data := generateSeasonalData(72, 24, 150, 1.0)
	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict with damped model error: %v", err)
	}
	if len(result.Forecasts) != 6 {
		t.Errorf("expected 6 forecast points, got %d", len(result.Forecasts))
	}
}

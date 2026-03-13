package integration_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// TestHoltWintersDamped_DampingSum exercises the dampingSum code path
// by using TrendDamped configuration.
func TestHoltWintersDamped_DampingSum(t *testing.T) {
	cfg := prediction.DefaultConfig()
	cfg.TrendType = prediction.TrendDamped
	cfg.SeasonalType = prediction.SeasonalAdditive
	cfg.DampingFactor = 0.95
	cfg.SeasonalPeriod = 24

	hw := prediction.NewHoltWintersWithConfig(cfg)

	n := 96 // 4 periods
	data := make([]float64, n)
	for i := range data {
		phase := float64(i%24) / 24.0 * 6.28318
		data[i] = 500 + 100*sinApprox(phase) + float64(i)*0.2
	}

	if err := hw.Fit(data); err != nil {
		t.Fatalf("HoltWinters Fit error: %v", err)
	}

	// Predict — triggers dampingSum since TrendType=TrendDamped
	result, err := hw.Predict(12)
	if err != nil {
		t.Fatalf("HoltWinters Predict error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ForecastResult")
	}
	if len(result.Forecasts) != 12 {
		t.Errorf("expected 12 forecast values, got %d", len(result.Forecasts))
	}
}

// TestHoltWintersDamped_MultiplicativeSeasonal exercises multiplicative seasonal with damped trend.
func TestHoltWintersDamped_MultiplicativeSeasonal(t *testing.T) {
	cfg := prediction.DefaultConfig()
	cfg.TrendType = prediction.TrendDamped
	cfg.SeasonalType = prediction.SeasonalMultiplicative
	cfg.DampingFactor = 0.9
	cfg.SeasonalPeriod = 12

	hw := prediction.NewHoltWintersWithConfig(cfg)

	n := 48
	data := make([]float64, n)
	for i := range data {
		phase := float64(i%12) / 12.0 * 6.28318
		data[i] = 100 * (1 + 0.3*sinApprox(phase)) * (1 + float64(i)*0.01)
	}

	if err := hw.Fit(data); err != nil {
		t.Fatalf("HoltWinters Fit error: %v", err)
	}

	result, err := hw.Predict(6)
	if err != nil {
		t.Fatalf("HoltWinters Predict error: %v", err)
	}
	_ = result
}

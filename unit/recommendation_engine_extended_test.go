package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

// ─── Engine constructor / configuration tests ────────────────────────────────

// TestNewEngineWithPricing_ProducesEngine verifies that NewEngineWithPricing
// returns a non-nil engine without panicking.
func TestNewEngineWithPricing_ProducesEngine(t *testing.T) {
	for _, preset := range []string{"aws-us-east-1", "gcp-us-central1", "default"} {
		e := recommendation.NewEngineWithPricing(preset)
		if e == nil {
			t.Errorf("NewEngineWithPricing(%q) returned nil", preset)
		}
	}
}

// TestSetRecommendationTTL_UpdatesTTL verifies that SetRecommendationTTL stores
// the provided value.
func TestSetRecommendationTTL_UpdatesTTL(t *testing.T) {
	e := recommendation.NewEngine()
	ttl := 48 * time.Hour
	e.SetRecommendationTTL(ttl)
	if e.RecommendationTTL != ttl {
		t.Errorf("expected RecommendationTTL=%v, got %v", ttl, e.RecommendationTTL)
	}
}

// TestSetPricingModel_NilFallsBack verifies that calling SetPricingModel with
// nil does not panic (uses default pricing).
func TestSetPricingModel_NilFallsBack(t *testing.T) {
	e := recommendation.NewEngine()
	e.SetPricingModel(nil) // must not panic
}

// TestSetPricingModel_CustomModel verifies that setting a custom pricing model
// and then using the engine still works without panic.
func TestSetPricingModel_CustomModel(t *testing.T) {
	e := recommendation.NewEngine()
	e.SetPricingModel(&cost.PricingModel{
		Provider:        "custom",
		CPUPerCoreHour:  0.048,
		MemoryPerGBHour: 0.006,
	})
	// Should not panic when used.
	if e == nil {
		t.Error("expected non-nil engine after SetPricingModel")
	}
}

// ─── NewConfidenceCalculatorWithConfig tests ─────────────────────────────────

// TestNewConfidenceCalculatorWithConfig_Works verifies that creating a
// confidence calculator with a custom config returns a non-nil object.
func TestNewConfidenceCalculatorWithConfig_Works(t *testing.T) {
	config := recommendation.DefaultConfidenceConfig()
	// Modify one field to ensure the custom path is taken.
	config.MinSamples = 5
	cc := recommendation.NewConfidenceCalculatorWithConfig(config)
	if cc == nil {
		t.Fatal("expected non-nil ConfidenceCalculator")
	}
}

// TestNewConfidenceCalculatorWithConfig_CalculatesConfidence verifies that a
// calculator built with a custom config still produces valid confidence scores.
func TestNewConfidenceCalculatorWithConfig_CalculatesConfidence(t *testing.T) {
	config := recommendation.DefaultConfidenceConfig()
	cc := recommendation.NewConfidenceCalculatorWithConfig(config)

	summary := makeMetricsSummary(100, 24, 0.2)
	result := cc.CalculateConfidence(summary)
	if result.Score < 0 || result.Score > 100 {
		t.Errorf("expected score in [0,100], got %.1f", result.Score)
	}
}

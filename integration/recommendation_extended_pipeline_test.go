package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

// TestRecommendationExtended_SetPricingModel verifies SetPricingModel on the engine.
func TestRecommendationExtended_SetPricingModel(t *testing.T) {
	eng := recommendation.NewEngine()

	pricing := &cost.PricingModel{
		Provider:        "aws",
		Region:          "us-east-1",
		CPUPerCoreHour:  0.048,
		MemoryPerGBHour: 0.006,
	}
	eng.SetPricingModel(pricing)

	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   95,
				MinSamples:      10,
				SafetyMargin:    1.0,
				HistoryDuration: "24h",
			},
		},
	}
	st := populatedStorage(200, 200, 300, 128*1024*1024, 128*1024*1024, 24*time.Hour)
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations after SetPricingModel error: %v", err)
	}
	_ = recs
}

// TestRecommendationExtended_ConfidenceWithConfig verifies NewConfidenceCalculatorWithConfig.
func TestRecommendationExtended_ConfidenceWithConfig(t *testing.T) {
	cfg := recommendation.DefaultConfidenceConfig()
	cfg.MinSamples = 5
	cfg.IdealSamples = 200

	calc := recommendation.NewConfidenceCalculatorWithConfig(cfg)
	if calc == nil {
		t.Fatal("expected non-nil ConfidenceCalculator")
	}
}

// TestRecommendationExtended_FormatScoreBreakdown verifies FormatScoreBreakdown output.
func TestRecommendationExtended_FormatScoreBreakdown(t *testing.T) {
	calc := recommendation.NewConfidenceCalculator()

	summary := recommendation.MetricsSummary{
		SampleCount:  50,
		OldestSample: time.Now().Add(-24 * time.Hour),
		NewestSample: time.Now(),
	}
	score := calc.CalculateConfidence(summary)
	breakdown := score.FormatScoreBreakdown()
	if breakdown == "" {
		t.Error("expected non-empty score breakdown")
	}
}

// TestRecommendationExtended_MultiContainerEngine verifies engine with pods having multiple containers.
func TestRecommendationExtended_MultiContainerEngine(t *testing.T) {
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   90,
				MinSamples:      5,
				SafetyMargin:    1.0,
				HistoryDuration: "12h",
			},
		},
	}

	// Use more samples and higher CPU values to trigger recommendations
	st := populatedStorage(300, 500, 1000, 256*1024*1024, 512*1024*1024, 12*time.Hour)
	eng := recommendation.NewEngineWithPricing("default")
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations multi-container error: %v", err)
	}
	_ = recs
}

// TestRecommendationExtended_PerformanceStrategy verifies Performance strategy recommendations.
func TestRecommendationExtended_PerformanceStrategy(t *testing.T) {
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyConservative,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   99,
				MinSamples:      10,
				SafetyMargin:    1.2,
				HistoryDuration: "24h",
			},
		},
	}
	st := populatedStorage(200, 200, 300, 128*1024*1024, 128*1024*1024, 24*time.Hour)
	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations performance strategy error: %v", err)
	}
	_ = recs
}

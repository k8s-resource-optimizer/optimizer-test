package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/safety"
)

// TestCoverageBoost_RecommendationWithThresholds exercises parseCPUToMillicores and parseMemoryToBytes
// via GenerateRecommendations with ResourceThresholds configured.
func TestCoverageBoost_RecommendationWithThresholds(t *testing.T) {
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
			ResourceThresholds: &v1alpha1.ResourceThresholds{
				CPU: &v1alpha1.ResourceLimit{
					Min: "50m",
					Max: "2",
				},
				Memory: &v1alpha1.ResourceLimit{
					Min: "64Mi",
					Max: "4Gi",
				},
			},
		},
	}

	st := populatedStorage(200, 200, 300, 128*1024*1024, 128*1024*1024, 24*time.Hour)
	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations with thresholds error: %v", err)
	}
	_ = recs
}

// TestCoverageBoost_AggressiveStrategy tests aggressive strategy to exercise shouldSwap path.
func TestCoverageBoost_AggressiveStrategy(t *testing.T) {
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyAggressive,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   90,
				MinSamples:      10,
				SafetyMargin:    1.0,
				HistoryDuration: "24h",
			},
		},
	}

	st := populatedStorage(200, 200, 300, 128*1024*1024, 128*1024*1024, 24*time.Hour)
	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations aggressive error: %v", err)
	}
	_ = recs
}

// TestCoverageBoost_OOMPriorityString verifies OOMPriority.String method.
func TestCoverageBoost_OOMPriorityString(t *testing.T) {
	priorities := []safety.OOMPriority{
		safety.OOMPriorityNone,
		safety.OOMPriorityLow,
		safety.OOMPriorityMedium,
		safety.OOMPriorityHigh,
		safety.OOMPriorityCritical,
	}

	for _, p := range priorities {
		str := p.String()
		if str == "" {
			t.Errorf("expected non-empty string for priority %d", p)
		}
	}
}

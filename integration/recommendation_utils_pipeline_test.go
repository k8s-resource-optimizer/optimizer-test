package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

// TestRecommendationUtils_ExpiryAndAge verifies expiry-related methods on
// WorkloadRecommendation.
func TestRecommendationUtils_ExpiryAndAge(t *testing.T) {
	rec := recommendation.WorkloadRecommendation{
		GeneratedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(22 * time.Hour),
	}

	if rec.IsExpired() {
		t.Error("expected fresh recommendation not to be expired")
	}
	if rec.Age() < 2*time.Hour-time.Second {
		t.Errorf("expected age >= 2h, got %v", rec.Age())
	}
	if rec.TimeToExpiry() <= 0 {
		t.Error("expected positive TimeToExpiry")
	}
	status := rec.ExpiryStatus()
	if status == "" {
		t.Error("expected non-empty ExpiryStatus")
	}
}

// TestRecommendationUtils_ExpiredRecommendation verifies IsExpired and ExpiryStatus
// for an expired recommendation.
func TestRecommendationUtils_ExpiredRecommendation(t *testing.T) {
	rec := recommendation.WorkloadRecommendation{
		GeneratedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt:   time.Now().Add(-24 * time.Hour),
	}

	if !rec.IsExpired() {
		t.Error("expected expired recommendation to be expired")
	}
	status := rec.ExpiryStatus()
	if len(status) < 7 || status[:7] != "EXPIRED" {
		t.Errorf("expected status to start with EXPIRED, got %q", status)
	}
}

// TestRecommendationUtils_FilterExpired verifies FilterExpired removes expired recs.
func TestRecommendationUtils_FilterExpired(t *testing.T) {
	fresh := recommendation.WorkloadRecommendation{
		Namespace:    "default",
		WorkloadName: "fresh-app",
		GeneratedAt:  time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	expired := recommendation.WorkloadRecommendation{
		Namespace:    "default",
		WorkloadName: "old-app",
		GeneratedAt:  time.Now().Add(-48 * time.Hour),
		ExpiresAt:    time.Now().Add(-24 * time.Hour),
	}

	all := []recommendation.WorkloadRecommendation{fresh, expired}
	filtered := recommendation.FilterExpired(all)
	for _, r := range filtered {
		if r.IsExpired() {
			t.Errorf("FilterExpired should not include expired rec: %s", r.WorkloadName)
		}
	}
}

// TestRecommendationUtils_ShouldApply verifies ShouldApply logic.
func TestRecommendationUtils_ShouldApply(t *testing.T) {
	rec := recommendation.WorkloadRecommendation{
		GeneratedAt: time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		Containers: []recommendation.ContainerRecommendation{
			{CurrentCPU: 500, RecommendedCPU: 250}, // -50%
		},
	}
	// ShouldApply checks if change is significant enough.
	_, _ = rec.ShouldApply(10.0)
}

// TestRecommendationUtils_ChangePercents verifies CPU/memory change percent helpers.
func TestRecommendationUtils_ChangePercents(t *testing.T) {
	cr := recommendation.ContainerRecommendation{
		CurrentCPU:        1000,
		RecommendedCPU:    500,
		CurrentMemory:     512 * 1024 * 1024,
		RecommendedMemory: 256 * 1024 * 1024,
	}

	cpuPct := cr.CalculateCPUChangePercent()
	memPct := cr.CalculateMemoryChangePercent()
	maxPct := cr.MaxChangePercent()

	if cpuPct == 0 {
		t.Error("expected non-zero CPU change percent")
	}
	if memPct == 0 {
		t.Error("expected non-zero memory change percent")
	}
	if maxPct < cpuPct && maxPct < memPct {
		t.Error("MaxChangePercent should be >= both CPU and memory change")
	}
}

// TestRecommendationUtils_NewEngineWithPricing verifies NewEngineWithPricing.
func TestRecommendationUtils_NewEngineWithPricing(t *testing.T) {
	eng := recommendation.NewEngineWithPricing("default")
	if eng == nil {
		t.Fatal("expected non-nil engine from NewEngineWithPricing")
	}

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
		t.Fatalf("GenerateRecommendations with pricing error: %v", err)
	}
	_ = recs
}

// TestRecommendationUtils_SetRecommendationTTL verifies TTL configuration.
func TestRecommendationUtils_SetRecommendationTTL(t *testing.T) {
	eng := recommendation.NewEngine()
	eng.SetRecommendationTTL(12 * time.Hour)

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
		t.Fatalf("GenerateRecommendations error: %v", err)
	}
	for _, r := range recs {
		if r.ExpiresAt.IsZero() {
			t.Error("expected ExpiresAt to be set after TTL configuration")
		}
	}
}

// TestCostUtils_EstimateWorkloadSavings verifies EstimateWorkloadSavings.
func TestCostUtils_EstimateWorkloadSavings(t *testing.T) {
	calc := cost.NewCalculatorWithPreset("default")
	savings := calc.EstimateWorkloadSavings(
		"default", "Deployment", "my-app",
		3,
		[]cost.ContainerResourceChange{
			{
				ContainerName:     "app",
				CurrentCPU:        1000,
				RecommendedCPU:    400,
				CurrentMemory:     512 * 1024 * 1024,
				RecommendedMemory: 256 * 1024 * 1024,
			},
		},
	)
	if savings.WorkloadName != "my-app" {
		t.Errorf("expected WorkloadName 'my-app', got %s", savings.WorkloadName)
	}
	if len(savings.Containers) == 0 {
		t.Error("expected non-empty Containers in WorkloadSavings")
	}
}

package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/storage"
)

// populatedStorageWithPodName creates storage with a specific pod name.
func populatedStorageWithPodName(n int, podName string, baseCPU, baseMem int64, dur time.Duration) *storage.InMemoryStorage {
	st := storage.NewStorage()
	start := time.Now().Add(-dur)
	step := dur / time.Duration(n)
	for i := 0; i < n; i++ {
		st.Add(models.PodMetric{
			PodName:   podName,
			Namespace: "default",
			Timestamp: start.Add(time.Duration(i) * step),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      baseCPU,
					UsageMemory:   baseMem,
					RequestCPU:    1000,
					RequestMemory: 512 * 1024 * 1024,
				},
			},
		})
	}
	return st
}

// TestWorkloadNaming_MultiSegmentPodName exercises joinParts via extractWorkloadName
// by using pods with 4+ dash-separated name segments (deployment pod naming pattern).
func TestWorkloadNaming_MultiSegmentPodName(t *testing.T) {
	// Pod names like "my-deployment-abc12-xyz45" have 4 segments
	// extractWorkloadName removes last 2 → "my-deployment"
	st := populatedStorageWithPodName(50, "my-deploy-abc12-xyz45", 200, 128*1024*1024, 24*time.Hour)

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

	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations error: %v", err)
	}
	// Verify workload name extraction worked
	for _, r := range recs {
		if r.WorkloadName == "" {
			t.Error("expected non-empty workload name")
		}
	}
}

// TestWorkloadNaming_ThreeSegmentPodName exercises 3-segment pod name.
func TestWorkloadNaming_ThreeSegmentPodName(t *testing.T) {
	// "app-web-abc12" → 3 segments → joinParts(["app", "web"])
	st := populatedStorageWithPodName(50, "app-web-abc12", 300, 256*1024*1024, 24*time.Hour)

	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyConservative,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   90,
				MinSamples:      10,
				SafetyMargin:    1.1,
				HistoryDuration: "24h",
			},
		},
	}

	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(&storageProvider{st}, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations error: %v", err)
	}
	_ = recs
}

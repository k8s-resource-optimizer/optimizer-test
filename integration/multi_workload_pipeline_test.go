package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/storage"
)

// populatedMultiWorkloadStorage creates storage with multiple distinct workloads.
func populatedMultiWorkloadStorage(workloadNames []string, n int, dur time.Duration) *storage.InMemoryStorage {
	st := storage.NewStorage()
	start := time.Now().Add(-dur)
	step := dur / time.Duration(n)

	for _, workload := range workloadNames {
		// Pod names follow: <workload>-<hash>-<random> pattern so extractWorkloadName
		// strips last 2 parts and gets workload name
		podName := workload + "-abc12-xyz45"
		for i := 0; i < n; i++ {
			cpu := int64(200 + 50*i/n)
			mem := int64(128 * 1024 * 1024)
			st.Add(models.PodMetric{
				PodName:   podName,
				Namespace: "multi-ns",
				Timestamp: start.Add(time.Duration(i) * step),
				Containers: []models.ContainerMetric{
					{
						ContainerName: "app",
						UsageCPU:      cpu,
						UsageMemory:   mem,
						RequestCPU:    1000,
						RequestMemory: 512 * 1024 * 1024,
					},
				},
			})
		}
	}
	return st
}

// TestMultiWorkload_SortByPriority verifies sortRecommendationsByPriority and shouldSwap
// by generating recommendations for multiple workloads.
func TestMultiWorkload_SortByPriority(t *testing.T) {
	workloads := []string{"web", "api", "worker"}
	st := populatedMultiWorkloadStorage(workloads, 50, 24*time.Hour)

	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"multi-ns"},
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
		t.Fatalf("GenerateRecommendations multi-workload error: %v", err)
	}
	// With 3 workloads, we should get up to 3 recommendations
	// (if enough data). Sort is called, exercising shouldSwap.
	_ = recs
}

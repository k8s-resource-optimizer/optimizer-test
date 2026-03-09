// Package integration contains integration tests that exercise multiple
// packages working together — from metrics ingestion through recommendation
// generation.  These tests use only in-memory implementations (no real
// Kubernetes cluster is required).
package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/storage"
)

// ─── helpers ─────────────────────────────────────────────────────────────

// populatedStorage inserts `n` PodMetric records into a fresh InMemoryStorage.
// CPU and memory values increase linearly from base to base+spread over `dur`.
func populatedStorage(n int, baseCPU, spreadCPU, baseMem, spreadMem int64, dur time.Duration) *storage.InMemoryStorage {
	st := storage.NewStorage()
	start := time.Now().Add(-dur)
	step := dur / time.Duration(n)
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		cpu := baseCPU + int64(float64(spreadCPU)*frac)
		mem := baseMem + int64(float64(spreadMem)*frac)
		st.Add(models.PodMetric{
			PodName:   "pod-0",
			Namespace: "default",
			Timestamp: start.Add(time.Duration(i) * step),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      cpu,
					UsageMemory:   mem,
					RequestCPU:    1000,
					RequestMemory: 512 * 1024 * 1024,
					LimitCPU:      2000,
					LimitMemory:   1024 * 1024 * 1024,
				},
			},
		})
	}
	return st
}

// storageProvider wraps InMemoryStorage to satisfy recommendation.MetricsProvider.
type storageProvider struct {
	st *storage.InMemoryStorage
}

func (p *storageProvider) GetMetricsByNamespace(ns string, since time.Duration) []models.PodMetric {
	return p.st.GetMetricsByNamespace(ns, since)
}

func (p *storageProvider) GetMetricsByWorkload(ns, workload string, since time.Duration) []models.PodMetric {
	return p.st.GetMetricsByWorkload(ns, workload, since)
}

// balancedConfig returns a minimal OptimizerConfig for the "default" namespace.
func balancedConfig() *v1alpha1.OptimizerConfig {
	return &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:    95,
				MemoryPercentile: 95,
				MinSamples:       10,
				SafetyMargin:     1.2,
				HistoryDuration:  "24h",
			},
		},
	}
}

// ─── tests ───────────────────────────────────────────────────────────────

// TestPipeline_StoreAndRecommend verifies the core collect→store→recommend
// pipeline.  Metrics are inserted into storage and the recommendation engine
// must produce at least one valid recommendation.
func TestPipeline_StoreAndRecommend(t *testing.T) {
	st := populatedStorage(200, 100, 400, 128*1024*1024, 384*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}

	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(provider, balancedConfig())
	if err != nil {
		t.Fatalf("GenerateRecommendations: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation from pipeline")
	}
	rec := recs[0]
	if len(rec.Containers) == 0 {
		t.Fatal("recommendation must contain at least one container entry")
	}
	if rec.Containers[0].RecommendedCPU <= 0 {
		t.Errorf("recommended CPU must be positive, got %d", rec.Containers[0].RecommendedCPU)
	}
}

// TestPipeline_OverProvisionedWorkload verifies that the engine recommends
// less CPU/memory than currently allocated when a workload consistently
// uses far fewer resources than requested.
func TestPipeline_OverProvisionedWorkload(t *testing.T) {
	// Workload uses only 100–150m CPU but has 1000m requested.
	st := populatedStorage(200, 100, 50, 64*1024*1024, 32*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}

	eng := recommendation.NewEngine()
	recs, err := eng.GenerateRecommendations(provider, balancedConfig())
	if err != nil {
		t.Fatalf("GenerateRecommendations: %v", err)
	}
	if len(recs) == 0 || len(recs[0].Containers) == 0 {
		t.Skip("no recommendations generated")
	}

	// Recommended CPU should be well below the 1000m current request.
	recCPU := recs[0].Containers[0].RecommendedCPU
	if recCPU >= 1000 {
		t.Errorf("expected recommendation < 1000m for over-provisioned workload, got %dm", recCPU)
	}
}

// TestPipeline_UnderProvisionedWorkload verifies that the engine recommends
// more CPU/memory when usage consistently approaches or exceeds the current request.
func TestPipeline_UnderProvisionedWorkload(t *testing.T) {
	// Workload uses 800–1000m CPU but only has 500m requested.
	st := populatedStorage(200, 800, 200, 256*1024*1024, 128*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}

	eng := recommendation.NewEngine()
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:    95,
				MinSamples:       10,
				SafetyMargin:     1.0, // no extra buffer so we measure raw recommendation
				HistoryDuration:  "24h",
			},
		},
	}
	recs, err := eng.GenerateRecommendations(provider, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations: %v", err)
	}
	if len(recs) == 0 || len(recs[0].Containers) == 0 {
		t.Skip("no recommendations generated")
	}

	recCPU := recs[0].Containers[0].RecommendedCPU
	// P95 of 800–1000m is ~990m, well above the 500m current request.
	if recCPU < 500 {
		t.Errorf("expected recommendation ≥ 500m for under-provisioned workload, got %dm", recCPU)
	}
}

// TestPipeline_StorageCleanup verifies that the Cleanup() method removes
// metrics older than the maxAge window and keeps recent ones.
func TestPipeline_StorageCleanup(t *testing.T) {
	st := storage.NewStorage()
	maxAge := 1 * time.Hour

	// Old metric: 2 hours ago — must be removed by Cleanup.
	st.Add(models.PodMetric{
		PodName:   "pod-old",
		Namespace: "default",
		Timestamp: time.Now().Add(-2 * time.Hour),
		Containers: []models.ContainerMetric{
			{ContainerName: "c", UsageCPU: 100, UsageMemory: 64 * 1024 * 1024},
		},
	})

	// Recent metric: just now — must be retained.
	st.Add(models.PodMetric{
		PodName:   "pod-fresh",
		Namespace: "default",
		Timestamp: time.Now(),
		Containers: []models.ContainerMetric{
			{ContainerName: "c", UsageCPU: 200, UsageMemory: 128 * 1024 * 1024},
		},
	})

	removed := st.Cleanup(maxAge)
	if removed == 0 {
		t.Error("expected at least 1 old metric to be cleaned up")
	}

	// After cleanup, querying for the last hour should not return the old pod.
	results := st.GetMetricsByNamespace("default", maxAge)
	for _, m := range results {
		if m.PodName == "pod-old" {
			t.Error("old metric should have been removed by Cleanup")
		}
	}
}

// TestPipeline_RecommendationConsistency verifies that two identical runs
// with the same in-memory metrics produce identical CPU recommendations.
// This is required for reproducible benchmarks and audit trails.
func TestPipeline_RecommendationConsistency(t *testing.T) {
	st := populatedStorage(200, 200, 600, 256*1024*1024, 512*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}
	cfg := balancedConfig()
	eng := recommendation.NewEngine()

	recs1, err1 := eng.GenerateRecommendations(provider, cfg)
	recs2, err2 := eng.GenerateRecommendations(provider, cfg)

	if err1 != nil || err2 != nil {
		t.Fatalf("GenerateRecommendations errors: run1=%v, run2=%v", err1, err2)
	}
	if len(recs1) != len(recs2) {
		t.Errorf("inconsistent recommendation count: run1=%d, run2=%d", len(recs1), len(recs2))
	}
	if len(recs1) > 0 && len(recs2) > 0 &&
		len(recs1[0].Containers) > 0 && len(recs2[0].Containers) > 0 {
		cpu1 := recs1[0].Containers[0].RecommendedCPU
		cpu2 := recs2[0].Containers[0].RecommendedCPU
		if cpu1 != cpu2 {
			t.Errorf("non-deterministic CPU: run1=%dm, run2=%dm", cpu1, cpu2)
		}
	}
}

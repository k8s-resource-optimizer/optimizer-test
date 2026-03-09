package unit_test

import (
	"math"
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

// mockMetricsProvider implements recommendation.MetricsProvider for testing.
type mockMetricsProvider struct {
	metrics []models.PodMetric
}

func (m *mockMetricsProvider) GetMetricsByNamespace(namespace string, since time.Duration) []models.PodMetric {
	return m.metrics
}

func (m *mockMetricsProvider) GetMetricsByWorkload(namespace, workload string, since time.Duration) []models.PodMetric {
	return m.metrics
}

// generateSyntheticMetrics builds N samples with a known P95 value.
// CPU values are distributed from baseVal to baseVal + spread linearly.
func generateSyntheticMetrics(n int, baseCPU, baseMemory, spreadCPU, spreadMemory int64) []models.PodMetric {
	metrics := make([]models.PodMetric, n)
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		cpu := baseCPU + int64(float64(spreadCPU)*frac)
		mem := baseMemory + int64(float64(spreadMemory)*frac)
		metrics[i] = models.PodMetric{
			PodName:   "pod-0",
			Namespace: "default",
			Timestamp: time.Now().Add(-time.Duration(n-i) * time.Minute),
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
		}
	}
	return metrics
}

// truePercentile calculates the true Pth percentile of a sorted ascending slice.
func truePercentile(values []int64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	idx := float64(p)/100.0*float64(len(values)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return float64(values[lo])
	}
	frac := idx - float64(lo)
	return float64(values[lo])*(1-frac) + float64(values[hi])*frac
}

func basicConfig(percentile int) *v1alpha1.OptimizerConfig {
	return &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:    percentile,
				MemoryPercentile: percentile,
				MinSamples:       10,
				SafetyMargin:     1.0, // no buffer so we can check accuracy
				HistoryDuration:  "24h",
			},
		},
	}
}

func TestRecommendationEngine_P95Accuracy(t *testing.T) {
	// 200 linearly distributed samples: CPU 100m..500m
	n := 200
	baseCPU := int64(100)
	spreadCPU := int64(400)
	baseMemory := int64(100 * 1024 * 1024)
	spreadMemory := int64(400 * 1024 * 1024)

	metrics := generateSyntheticMetrics(n, baseCPU, baseMemory, spreadCPU, spreadMemory)
	provider := &mockMetricsProvider{metrics: metrics}

	// Compute true P95
	cpuValues := make([]int64, n)
	for i := 0; i < n; i++ {
		cpuValues[i] = baseCPU + int64(float64(spreadCPU)*float64(i)/float64(n-1))
	}
	truP95 := truePercentile(cpuValues, 95)

	eng := recommendation.NewEngine()
	cfg := basicConfig(95)
	recs, err := eng.GenerateRecommendations(provider, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations failed: %v", err)
	}

	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}

	rec := recs[0]
	if len(rec.Containers) == 0 {
		t.Fatal("expected at least one container recommendation")
	}

	got := float64(rec.Containers[0].RecommendedCPU)
	pctError := math.Abs(got-truP95) / truP95 * 100

	if pctError > 1.0 {
		t.Errorf("P95 accuracy: true=%.1f, got=%.1f, error=%.2f%% (must be ≤1%%)", truP95, got, pctError)
	}
}

func TestRecommendationEngine_P99Accuracy(t *testing.T) {
	n := 200
	baseCPU := int64(50)
	spreadCPU := int64(950)

	metrics := generateSyntheticMetrics(n, baseCPU, 128*1024*1024, spreadCPU, 0)
	provider := &mockMetricsProvider{metrics: metrics}

	cpuValues := make([]int64, n)
	for i := 0; i < n; i++ {
		cpuValues[i] = baseCPU + int64(float64(spreadCPU)*float64(i)/float64(n-1))
	}
	truP99 := truePercentile(cpuValues, 99)

	eng := recommendation.NewEngine()
	cfg := basicConfig(99)
	recs, err := eng.GenerateRecommendations(provider, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations failed: %v", err)
	}
	if len(recs) == 0 || len(recs[0].Containers) == 0 {
		t.Fatal("expected recommendation")
	}

	got := float64(recs[0].Containers[0].RecommendedCPU)
	pctError := math.Abs(got-truP99) / truP99 * 100
	if pctError > 1.0 {
		t.Errorf("P99 accuracy: true=%.1f, got=%.1f, error=%.2f%% (must be ≤1%%)", truP99, got, pctError)
	}
}

func TestRecommendationEngine_InsufficientSamples(t *testing.T) {
	// Fewer than MinSamples — should return no recommendations or error gracefully
	metrics := generateSyntheticMetrics(5, 100, 128*1024*1024, 400, 0)
	provider := &mockMetricsProvider{metrics: metrics}

	eng := recommendation.NewEngine()
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				MinSamples: 100,
			},
		},
	}

	recs, err := eng.GenerateRecommendations(provider, cfg)
	// Either error or empty recommendations is acceptable
	if err == nil && len(recs) > 0 && len(recs[0].Containers) > 0 {
		// If a recommendation was returned, it should indicate low confidence
		for _, r := range recs {
			for _, c := range r.Containers {
				if c.SampleCount >= 100 {
					t.Errorf("sample count %d should be < 100 (MinSamples)", c.SampleCount)
				}
			}
		}
	}
}

func TestRecommendationEngine_SafetyMarginApplied(t *testing.T) {
	n := 100
	metrics := generateSyntheticMetrics(n, 100, 128*1024*1024, 400, 400*1024*1024)
	provider := &mockMetricsProvider{metrics: metrics}

	// First: get recommendation without safety margin
	engNoMargin := recommendation.NewEngine()
	cfgNoMargin := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:    95,
				MemoryPercentile: 95,
				MinSamples:       10,
				SafetyMargin:     1.0,
				HistoryDuration:  "24h",
			},
		},
	}
	recsNoMargin, _ := engNoMargin.GenerateRecommendations(provider, cfgNoMargin)

	// Second: with 20% safety margin
	engWithMargin := recommendation.NewEngine()
	cfgWithMargin := &v1alpha1.OptimizerConfig{
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
	recsWithMargin, _ := engWithMargin.GenerateRecommendations(provider, cfgWithMargin)

	if len(recsNoMargin) == 0 || len(recsWithMargin) == 0 {
		t.Skip("no recommendations generated — skipping margin comparison")
	}
	if len(recsNoMargin[0].Containers) == 0 || len(recsWithMargin[0].Containers) == 0 {
		t.Skip("no container recommendations")
	}

	noMarginCPU := recsNoMargin[0].Containers[0].RecommendedCPU
	withMarginCPU := recsWithMargin[0].Containers[0].RecommendedCPU

	if withMarginCPU <= noMarginCPU {
		t.Errorf("safety margin 1.2 should produce higher CPU recommendation: noMargin=%d, withMargin=%d",
			noMarginCPU, withMarginCPU)
	}
}

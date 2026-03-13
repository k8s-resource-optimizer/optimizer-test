package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/pareto"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

func TestParetoPipeline_RecommendationToOptimization(t *testing.T) {
	st := populatedStorage(200, 200, 400, 256*1024*1024, 256*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}

	eng := recommendation.NewEngine()
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

	recs, err := eng.GenerateRecommendations(provider, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations error: %v", err)
	}
	if len(recs) == 0 || len(recs[0].Containers) == 0 {
		t.Skip("no recommendations generated")
	}

	cr := recs[0].Containers[0]
	opt := pareto.NewOptimizer()

	p95CPU := cr.RecommendedCPU
	p95Mem := cr.RecommendedMemory
	avgCPU := p95CPU * 70 / 100
	avgMem := p95Mem * 70 / 100
	peakCPU := p95CPU * 130 / 100
	peakMem := p95Mem * 130 / 100

	solutions := opt.GenerateSolutionSet(
		recs[0].Namespace, recs[0].WorkloadName,
		cr.CurrentCPU, cr.CurrentMemory,
		avgCPU, avgMem,
		peakCPU, peakMem,
		p95CPU, p95Mem,
		peakCPU, peakMem,
		85.0,
	)

	if len(solutions) == 0 {
		t.Fatal("expected non-empty solution set from pareto optimizer")
	}

	result := opt.Optimize(solutions)
	if result == nil {
		t.Fatal("expected non-nil OptimizationResult")
	}
	if len(result.ParetoFrontier) == 0 {
		t.Error("expected non-empty ParetoFrontier")
	}
}

func TestParetoPipeline_BestSolutionForProfiles(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "my-app",
		int64(1000), int64(512*1024*1024),
		int64(300), int64(256*1024*1024),
		int64(900), int64(480*1024*1024),
		int64(700), int64(400*1024*1024),
		int64(850), int64(450*1024*1024),
		80.0,
	)

	result := opt.Optimize(solutions)
	if result == nil || len(result.ParetoFrontier) == 0 {
		t.Skip("no pareto frontier produced")
	}

	prodBest := opt.SelectBestForProfile(result, "production")
	devBest := opt.SelectBestForProfile(result, "development")

	if prodBest == nil {
		t.Error("expected non-nil best solution for production profile")
	}
	if devBest == nil {
		t.Error("expected non-nil best solution for development profile")
	}
}

func TestParetoPipeline_TradeOffsAnalysis(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "worker",
		int64(2000), int64(1024*1024*1024),
		int64(500), int64(512*1024*1024),
		int64(1800), int64(900*1024*1024),
		int64(1200), int64(700*1024*1024),
		int64(1600), int64(850*1024*1024),
		75.0,
	)

	result := opt.Optimize(solutions)
	if result == nil {
		t.Fatal("expected non-nil OptimizationResult")
	}
	if result.BestSolution == nil {
		t.Error("expected non-nil BestSolution")
	}
}

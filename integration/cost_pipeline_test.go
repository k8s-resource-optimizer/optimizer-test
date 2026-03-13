package integration_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

func TestCostPipeline_RecommendationToSavingsReport(t *testing.T) {
	st := populatedStorage(200, 100, 200, 128*1024*1024, 128*1024*1024, 24*time.Hour)
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
				SafetyMargin:    1.2,
				HistoryDuration: "24h",
			},
		},
	}
	recs, err := eng.GenerateRecommendations(provider, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations error: %v", err)
	}
	if len(recs) == 0 {
		t.Skip("no recommendations generated")
	}

	calc := cost.NewCalculatorWithPreset("default")
	var workloads []cost.WorkloadSavings

	for _, wr := range recs {
		var containers []cost.ContainerSavings
		for _, cr := range wr.Containers {
			savings := calc.EstimateSavings(
				cr.CurrentCPU, cr.RecommendedCPU,
				cr.CurrentMemory, cr.RecommendedMemory,
			)
			containers = append(containers, cost.ContainerSavings{
				ContainerName: cr.ContainerName,
				Namespace:     wr.Namespace,
				WorkloadName:  wr.WorkloadName,
				Savings:       savings,
				ReplicaCount:  1,
				TotalSavings:  savings,
			})
		}
		workloads = append(workloads, cost.WorkloadSavings{
			Namespace:    wr.Namespace,
			WorkloadKind: "Deployment",
			WorkloadName: wr.WorkloadName,
			ReplicaCount: 1,
			Containers:   containers,
		})
	}

	report := calc.GenerateReport(workloads)
	formatted := report.FormatReport()
	if formatted == "" {
		t.Error("expected non-empty FormatReport output")
	}
}

func TestCostPipeline_OverProvisionedSavingsPositive(t *testing.T) {
	st := populatedStorage(200, 50, 50, 64*1024*1024, 32*1024*1024, 24*time.Hour)
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
				SafetyMargin:    1.1,
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
	calc := cost.NewCalculatorWithPreset("default")
	savings := calc.EstimateSavings(
		cr.CurrentCPU, cr.RecommendedCPU,
		cr.CurrentMemory, cr.RecommendedMemory,
	)

	if savings.TotalSavingsPerHour < 0 {
		t.Errorf("expected positive savings for over-provisioned workload, got %.4f/hr",
			savings.TotalSavingsPerHour)
	}
}

func TestCostPipeline_MultiplePresets(t *testing.T) {
	cpu := int64(500)
	mem := int64(512 * 1024 * 1024)

	defaultCalc := cost.NewCalculatorWithPreset("default")
	awsCalc := cost.NewCalculatorWithPreset("aws-us-east-1")

	defaultCost := defaultCalc.CalculateCost(cpu, mem)
	awsCost := awsCalc.CalculateCost(cpu, mem)

	if defaultCost.TotalPerHour <= 0 {
		t.Errorf("expected positive cost from default preset, got %.6f", defaultCost.TotalPerHour)
	}
	if awsCost.TotalPerHour <= 0 {
		t.Errorf("expected positive cost from aws preset, got %.6f", awsCost.TotalPerHour)
	}
}

func TestCostPipeline_AggressiveVsConservativeSavings(t *testing.T) {
	st := populatedStorage(200, 100, 100, 128*1024*1024, 64*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}
	eng := recommendation.NewEngine()

	makeCfg := func(strategy v1alpha1.OptimizationStrategy) *v1alpha1.OptimizerConfig {
		return &v1alpha1.OptimizerConfig{
			Spec: v1alpha1.OptimizerConfigSpec{
				Enabled:          true,
				TargetNamespaces: []string{"default"},
				Strategy:         strategy,
				Recommendations: &v1alpha1.RecommendationConfig{
					CPUPercentile:   95,
					MinSamples:      10,
					SafetyMargin:    1.0,
					HistoryDuration: "24h",
				},
			},
		}
	}

	aggrRecs, err := eng.GenerateRecommendations(provider, makeCfg(v1alpha1.StrategyAggressive))
	if err != nil {
		t.Fatalf("aggressive error: %v", err)
	}
	consRecs, err := eng.GenerateRecommendations(provider, makeCfg(v1alpha1.StrategyConservative))
	if err != nil {
		t.Fatalf("conservative error: %v", err)
	}

	if len(aggrRecs) == 0 || len(consRecs) == 0 {
		t.Skip("no recommendations generated")
	}

	calc := cost.NewCalculatorWithPreset("default")
	aggrCPU := aggrRecs[0].Containers[0].RecommendedCPU
	consCPU := consRecs[0].Containers[0].RecommendedCPU

	aggrCost := calc.CalculateCost(aggrCPU, aggrRecs[0].Containers[0].RecommendedMemory)
	consCost := calc.CalculateCost(consCPU, consRecs[0].Containers[0].RecommendedMemory)

	if aggrCost.TotalPerHour > consCost.TotalPerHour {
		t.Errorf("aggressive cost (%.4f/hr) should be <= conservative cost (%.4f/hr)",
			aggrCost.TotalPerHour, consCost.TotalPerHour)
	}
}

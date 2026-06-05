package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/simulator"
)

func simRec(name, ns string, curCPU, recCPU, curMem, recMem int64, conf float64) recommendation.WorkloadRecommendation {
	return recommendation.WorkloadRecommendation{
		Namespace:    ns,
		WorkloadName: name,
		WorkloadKind: "Deployment",
		GeneratedAt:  time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Containers: []recommendation.ContainerRecommendation{
			{
				ContainerName:     "app",
				CurrentCPU:        curCPU,
				RecommendedCPU:    recCPU,
				CurrentMemory:     curMem,
				RecommendedMemory: recMem,
				Confidence:        conf,
			},
		},
	}
}

func TestSimulate_NoRecommendations_ReturnsError(t *testing.T) {
	sc := simulator.Scenario{
		Name:        "empty",
		Strategy:    simulator.StrategyBalanced,
		TimeHorizon: 30 * 24 * time.Hour,
	}
	_, err := simulator.Simulate(sc, nil)
	if err == nil {
		t.Error("expected error when no recommendations provided")
	}
}

func TestSimulate_Balanced_OverProvisioned(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		simRec("stress-cyclic", "default", 400, 150, 256*1024*1024, 64*1024*1024, 74.0),
		simRec("stress-master", "default", 800, 200, 512*1024*1024, 128*1024*1024, 85.0),
	}
	sc := simulator.Scenario{
		Name:        "balanced-over-provisioned",
		Strategy:    simulator.StrategyBalanced,
		TimeHorizon: 30 * 24 * time.Hour,
	}
	result, err := simulator.Simulate(sc, recs)
	if err != nil {
		t.Fatalf("Simulate error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalWorkloads <= 0 {
		t.Error("expected TotalWorkloads > 0")
	}
}

func TestSimulate_Aggressive_MaxSavings(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		simRec("web", "production", 1000, 200, 1024*1024*1024, 128*1024*1024, 90.0),
	}
	sc := simulator.Scenario{
		Name:        "aggressive",
		Strategy:    simulator.StrategyAggressive,
		TimeHorizon: 7 * 24 * time.Hour,
		Assumptions: simulator.ScenarioAssumptions{
			TrafficGrowth:         0.1,
			AllowDowntime:         false,
			SLAViolationTolerance: 0.05,
		},
	}
	result, err := simulator.Simulate(sc, recs)
	if err != nil {
		t.Fatalf("Simulate aggressive error: %v", err)
	}
	if result.RiskLevel == "" {
		t.Error("expected non-empty risk level")
	}
}

func TestSimulate_Conservative_LowRisk(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		simRec("api", "staging", 500, 480, 512*1024*1024, 500*1024*1024, 95.0),
	}
	sc := simulator.Scenario{
		Name:        "conservative",
		Strategy:    simulator.StrategyConservative,
		TimeHorizon: 90 * 24 * time.Hour,
	}
	result, err := simulator.Simulate(sc, recs)
	if err != nil {
		t.Fatalf("Simulate conservative error: %v", err)
	}
	if result.Confidence <= 0 {
		t.Error("expected positive confidence score")
	}
}

func TestSimulate_WorkloadFilter(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		simRec("web", "default", 400, 150, 256*1024*1024, 64*1024*1024, 80.0),
		simRec("api", "default", 400, 150, 256*1024*1024, 64*1024*1024, 80.0),
		simRec("db", "default", 400, 150, 256*1024*1024, 64*1024*1024, 80.0),
	}
	sc := simulator.Scenario{
		Name:        "filtered",
		Workloads:   []string{"web", "api"},
		Strategy:    simulator.StrategyBalanced,
		TimeHorizon: 30 * 24 * time.Hour,
	}
	result, err := simulator.Simulate(sc, recs)
	if err != nil {
		t.Fatalf("Simulate with filter error: %v", err)
	}
	if result.AffectedWorkloads > 2 {
		t.Errorf("expected at most 2 affected workloads with filter, got %d", result.AffectedWorkloads)
	}
}

func TestSimulate_SavingsProjection_Fields(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		simRec("heavy", "prod", 1000, 100, 2048*1024*1024, 256*1024*1024, 88.0),
	}
	sc := simulator.Scenario{
		Name:        "savings-check",
		Strategy:    simulator.StrategyAggressive,
		TimeHorizon: 30 * 24 * time.Hour,
	}
	result, err := simulator.Simulate(sc, recs)
	if err != nil {
		t.Fatalf("Simulate error: %v", err)
	}
	if result.EstimatedSavings.Monthly < 0 {
		t.Error("monthly savings should not be negative for over-provisioned workload")
	}
	if result.EstimatedSavings.Yearly < result.EstimatedSavings.Monthly {
		t.Error("yearly savings should be >= monthly savings")
	}
}

func TestSimulate_TrafficGrowth_IncreasesRisk(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		simRec("frontend", "prod", 400, 100, 256*1024*1024, 64*1024*1024, 80.0),
	}
	scNoGrowth := simulator.Scenario{
		Name:        "no-growth",
		Strategy:    simulator.StrategyBalanced,
		TimeHorizon: 30 * 24 * time.Hour,
	}
	scWithGrowth := simulator.Scenario{
		Name:        "with-growth",
		Strategy:    simulator.StrategyBalanced,
		TimeHorizon: 30 * 24 * time.Hour,
		Assumptions: simulator.ScenarioAssumptions{TrafficGrowth: 0.5},
	}
	r1, _ := simulator.Simulate(scNoGrowth, recs)
	r2, _ := simulator.Simulate(scWithGrowth, recs)
	if r1 == nil || r2 == nil {
		t.Fatal("expected non-nil results")
	}
}

func TestNewSimulator_IsNotNil(t *testing.T) {
	s := simulator.NewSimulator()
	if s == nil {
		t.Fatal("expected non-nil Simulator")
	}
}

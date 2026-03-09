package unit_test

import (
	"math"
	"testing"

	"intelligent-cluster-optimizer/pkg/pareto"
)

// TestGenerateSolutionSet_ProducesAtLeastSixSolutions checks the project-level
// requirement that the Pareto engine always generates ≥6 candidate solutions
// per reconciliation run so the optimizer has a meaningful solution space.
func TestGenerateSolutionSet_ProducesAtLeastSixSolutions(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "test-app",
		500, 256*1024*1024,   // current CPU (millicores), memory (bytes)
		300, 180*1024*1024,   // avg CPU, memory
		480, 250*1024*1024,   // peak CPU, memory
		400, 210*1024*1024,   // P95 CPU, memory
		460, 240*1024*1024,   // P99 CPU, memory
		85.0,                 // confidence
	)

	if len(solutions) < 6 {
		t.Errorf("expected at least 6 candidate solutions, got %d", len(solutions))
	}
}

func TestGenerateSolutionSet_SolutionIDs(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "api",
		1000, 512*1024*1024,
		600, 300*1024*1024,
		900, 450*1024*1024,
		750, 380*1024*1024,
		880, 420*1024*1024,
		90.0,
	)

	strategies := map[string]bool{}
	for _, s := range solutions {
		strategies[s.ID] = true
	}

	expected := []string{"conservative", "balanced", "aggressive", "cost-optimized", "performance", "current"}
	for _, name := range expected {
		if !strategies[name] {
			t.Errorf("expected solution with ID %q, not found", name)
		}
	}
}

func TestOptimize_ProducesNonEmptyFrontier(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "web-app",
		800, 512*1024*1024,
		500, 350*1024*1024,
		780, 510*1024*1024,
		650, 420*1024*1024,
		760, 490*1024*1024,
		88.0,
	)

	result := opt.Optimize(solutions)
	if len(result.ParetoFrontier) == 0 {
		t.Error("Pareto frontier must not be empty")
	}
}

func TestOptimize_FrontierSolutionsAreNonDominated(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "worker",
		600, 256*1024*1024,
		400, 180*1024*1024,
		590, 250*1024*1024,
		500, 220*1024*1024,
		570, 240*1024*1024,
		80.0,
	)

	result := opt.Optimize(solutions)

	// No frontier solution should be dominated by another frontier solution
	for i, a := range result.ParetoFrontier {
		for j, b := range result.ParetoFrontier {
			if i == j {
				continue
			}
			if b.Dominates(a) {
				t.Errorf("frontier solution %s is dominated by %s — frontier is invalid", a.ID, b.ID)
			}
		}
	}
}

func TestOptimize_BestSolutionIsOnFrontier(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"prod", "backend",
		1000, 1024*1024*1024,
		700, 700*1024*1024,
		980, 1000*1024*1024,
		800, 800*1024*1024,
		940, 950*1024*1024,
		92.0,
	)

	result := opt.Optimize(solutions)
	if result.BestSolution == nil {
		t.Fatal("best solution must not be nil")
	}

	found := false
	for _, s := range result.ParetoFrontier {
		if s.ID == result.BestSolution.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("best solution must be on the Pareto frontier")
	}
}

func TestOptimize_CrowdingDistancePositive(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"staging", "service",
		500, 256*1024*1024,
		350, 190*1024*1024,
		490, 250*1024*1024,
		420, 220*1024*1024,
		470, 240*1024*1024,
		85.0,
	)

	result := opt.Optimize(solutions)

	for _, s := range result.ParetoFrontier {
		if s.CrowdingDistance < 0 {
			t.Errorf("crowding distance for solution %s must be ≥ 0, got %f", s.ID, s.CrowdingDistance)
		}
	}
}

func TestOptimize_EmptyInputReturnsEmptyResult(t *testing.T) {
	opt := pareto.NewOptimizer()
	result := opt.Optimize(nil)

	if len(result.AllSolutions) != 0 {
		t.Error("empty input should produce empty result")
	}
}

func TestSelectBestForProfile_ReturnsValidSolution(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"prod", "api-gw",
		1000, 512*1024*1024,
		650, 320*1024*1024,
		990, 510*1024*1024,
		800, 420*1024*1024,
		960, 490*1024*1024,
		91.0,
	)
	result := opt.Optimize(solutions)

	profiles := []string{"production", "staging", "development", "performance", "unknown"}
	for _, p := range profiles {
		best := opt.SelectBestForProfile(result, p)
		if best == nil {
			t.Errorf("SelectBestForProfile(%q) returned nil", p)
		}
	}
}

func TestSolution_SetObjectives_AllPresent(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "test",
		500, 256*1024*1024,
		300, 200*1024*1024,
		480, 250*1024*1024,
		400, 220*1024*1024,
		460, 240*1024*1024,
		80.0,
	)

	for _, s := range solutions {
		if len(s.Objectives) == 0 {
			t.Errorf("solution %s has no objectives set", s.ID)
		}
	}
}

func TestOptimize_ConsistentResults(t *testing.T) {
	// Same input must produce same number of frontier solutions across runs
	opt := pareto.NewOptimizer()
	solutions1 := opt.GenerateSolutionSet(
		"default", "app",
		700, 384*1024*1024,
		450, 250*1024*1024,
		690, 380*1024*1024,
		560, 310*1024*1024,
		670, 360*1024*1024,
		87.0,
	)
	solutions2 := opt.GenerateSolutionSet(
		"default", "app",
		700, 384*1024*1024,
		450, 250*1024*1024,
		690, 380*1024*1024,
		560, 310*1024*1024,
		670, 360*1024*1024,
		87.0,
	)

	r1 := opt.Optimize(solutions1)
	r2 := opt.Optimize(solutions2)

	if len(r1.ParetoFrontier) != len(r2.ParetoFrontier) {
		t.Errorf("inconsistent results: run1 frontier=%d, run2 frontier=%d",
			len(r1.ParetoFrontier), len(r2.ParetoFrontier))
	}
}

func TestSolution_Dominates_Transitivity(t *testing.T) {
	// If A dominates B and B dominates C, then A should dominate C
	solA := pareto.NewSolution("a", "ns", "wl")
	solA.AddObjective(pareto.ObjectiveCost, "Cost", 1.0, 0.5, true)
	solA.AddObjective(pareto.ObjectivePerformance, "Perf", 100.0, 0.5, false)

	solB := pareto.NewSolution("b", "ns", "wl")
	solB.AddObjective(pareto.ObjectiveCost, "Cost", 2.0, 0.5, true)
	solB.AddObjective(pareto.ObjectivePerformance, "Perf", 90.0, 0.5, false)

	solC := pareto.NewSolution("c", "ns", "wl")
	solC.AddObjective(pareto.ObjectiveCost, "Cost", 3.0, 0.5, true)
	solC.AddObjective(pareto.ObjectivePerformance, "Perf", 80.0, 0.5, false)

	if !solA.Dominates(solB) {
		t.Error("A should dominate B")
	}
	if !solB.Dominates(solC) {
		t.Error("B should dominate C")
	}
	if !solA.Dominates(solC) {
		t.Error("A should dominate C (transitivity)")
	}
}

func TestSolution_NoDominance_TradeOff(t *testing.T) {
	solA := pareto.NewSolution("a", "ns", "wl")
	solA.AddObjective(pareto.ObjectiveCost, "Cost", 5.0, 0.5, true)
	solA.AddObjective(pareto.ObjectivePerformance, "Perf", 90.0, 0.5, false)

	solB := pareto.NewSolution("b", "ns", "wl")
	solB.AddObjective(pareto.ObjectiveCost, "Cost", 10.0, 0.5, true)
	solB.AddObjective(pareto.ObjectivePerformance, "Perf", 95.0, 0.5, false)

	if solA.Dominates(solB) {
		t.Error("A should not dominate B (trade-off)")
	}
	if solB.Dominates(solA) {
		t.Error("B should not dominate A (trade-off)")
	}
}

func TestSolution_OverallScore_Bounded(t *testing.T) {
	opt := pareto.NewOptimizer()
	solutions := opt.GenerateSolutionSet(
		"default", "svc",
		500, 256*1024*1024,
		300, 180*1024*1024,
		480, 250*1024*1024,
		400, 210*1024*1024,
		460, 240*1024*1024,
		80.0,
	)
	result := opt.Optimize(solutions)

	for _, s := range result.AllSolutions {
		if math.IsNaN(s.OverallScore) || math.IsInf(s.OverallScore, 0) {
			t.Errorf("solution %s has invalid overall score: %f", s.ID, s.OverallScore)
		}
	}
}

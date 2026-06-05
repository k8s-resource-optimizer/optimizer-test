package unit_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/metrics"
)

// TestPrometheusExporter_AtLeast20DistinctMetrics verifies WP7 success criterion:
// Prometheus must expose at least 20 distinct metrics.
func TestPrometheusExporter_AtLeast20DistinctMetrics(t *testing.T) {
	e := metrics.NewPrometheusExporter("wp7_check")

	// Count non-nil exported metric vectors in the struct (each corresponds to a named metric)
	count := 0
	if e.ReconciliationTotal != nil {
		count++
	}
	if e.ReconciliationDuration != nil {
		count++
	}
	if e.ReconciliationErrors != nil {
		count++
	}
	if e.CPURecommendation != nil {
		count++
	}
	if e.MemoryRecommendation != nil {
		count++
	}
	if e.ReplicaRecommendation != nil {
		count++
	}
	if e.SLAViolations != nil {
		count++
	}
	if e.HealthScore != nil {
		count++
	}
	if e.OptimizationBlocked != nil {
		count++
	}
	if e.RollbacksTriggered != nil {
		count++
	}
	if e.PolicyEvaluations != nil {
		count++
	}
	if e.PolicyBlockedChanges != nil {
		count++
	}
	if e.GitOpsExports != nil {
		count++
	}
	if e.GitOpsExportErrors != nil {
		count++
	}
	if e.PredictionsMade != nil {
		count++
	}
	if e.PeakLoadsPredicted != nil {
		count++
	}
	if e.ParetoFrontSize != nil {
		count++
	}
	if e.ParetoSolutions != nil {
		count++
	}
	if e.PodStartupDuration != nil {
		count++
	}
	if e.PodStartupDurationAvg != nil {
		count++
	}
	if e.PodStartupDurationP95 != nil {
		count++
	}
	if e.SlowStartupsTotal != nil {
		count++
	}

	if count < 20 {
		t.Errorf("WP7: expected at least 20 distinct Prometheus metrics, found %d", count)
	}
}

func TestPrometheusExporter_RecordReconciliation(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_recon2")
	e.RecordReconciliation("cfg1", "default", "success", 0.123)
	e.RecordReconciliation("cfg1", "default", "failure", 0.456)
}

func TestPrometheusExporter_RecordCPURecommendation(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_cpu2")
	e.RecordCPURecommendation("web", "default", "app", 400)
	e.RecordCPURecommendation("api", "production", "server", 800)
}

func TestPrometheusExporter_RecordMemoryRecommendation(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_mem2")
	e.RecordMemoryRecommendation("web", "default", "app", 512)
	e.RecordMemoryRecommendation("db", "production", "postgres", 2048)
}

func TestPrometheusExporter_RecordReplicaRecommendation(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_replica2")
	e.RecordReplicaRecommendation("web", "default", 3)
	e.RecordReplicaRecommendation("api", "production", 5)
}

func TestPrometheusExporter_RecordSLAViolation(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_sla2")
	e.RecordSLAViolation("cfg1", "default", "latency")
	e.RecordSLAViolation("cfg2", "production", "error_rate")
	e.RecordSLAViolation("cfg3", "staging", "availability")
}

func TestPrometheusExporter_RecordHealthScore(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_health2")
	e.RecordHealthScore("cfg1", "default", 95.5)
	e.RecordHealthScore("cfg2", "production", 72.3)
}

func TestPrometheusExporter_RecordPolicyEvaluation(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_policy2")
	e.RecordPolicyEvaluation("cfg1", "default", "no-reduce-cpu", "allow")
	e.RecordPolicyEvaluation("cfg1", "default", "require-approval-prod", "require_approval")
	e.RecordPolicyEvaluation("cfg1", "production", "deny-critical", "deny")
}

func TestPrometheusExporter_RecordPolicyBlockedChange(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_policyblock2")
	e.RecordPolicyBlockedChange("cfg1", "default", "no-reduce-cpu")
}

func TestPrometheusExporter_RecordGitOpsExport(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_gitops2")
	e.RecordGitOpsExport("cfg1", "default", "kustomize")
	e.RecordGitOpsExport("cfg2", "production", "helm")
}

func TestPrometheusExporter_RecordGitOpsExportError(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_gitopserr2")
	e.RecordGitOpsExportError("cfg1", "default", "kustomize")
}

func TestPrometheusExporter_RecordParetoFrontSize(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_pareto2")
	e.RecordParetoFrontSize("cfg1", "default", 6)
	e.RecordParetoFrontSize("cfg1", "default", 8)
}

func TestPrometheusExporter_RecordParetoSolution(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_paretosol2")
	e.RecordParetoSolution("cfg1", "default")
}

func TestPrometheusExporter_RecordPodStartupDuration(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_startup2")
	e.RecordPodStartupDuration("web", "default", "app", 1.23)
	e.RecordPodStartupDuration("api", "production", "server", 0.85)
}

func TestPrometheusExporter_RecordPodStartupDurationAvg(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_startupdavg2")
	e.RecordPodStartupDurationAvg("web", "default", 1.1)
}

func TestPrometheusExporter_RecordPodStartupDurationP95(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_startupp95_2")
	e.RecordPodStartupDurationP95("web", "default", 2.5)
}

func TestPrometheusExporter_RecordSlowStartup(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_slow2")
	e.RecordSlowStartup("web", "default", 5.0)
}

func TestPrometheusExporter_AllRecordMethods_NoPanic(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_all_methods")

	e.RecordReconciliation("c", "n", "success", 0.1)
	e.RecordReconciliationError("c", "n", "timeout")
	e.RecordCPURecommendation("w", "n", "app", 200)
	e.RecordMemoryRecommendation("w", "n", "app", 256)
	e.RecordReplicaRecommendation("w", "n", 2)
	e.RecordSLAViolation("c", "n", "latency")
	e.RecordHealthScore("c", "n", 88.0)
	e.RecordOptimizationBlocked("c", "n", "circuit_open")
	e.RecordRollback("c", "n", "oom")
	e.RecordPolicyEvaluation("c", "n", "pol1", "allow")
	e.RecordPolicyBlockedChange("c", "n", "pol1")
	e.RecordGitOpsExport("c", "n", "kustomize")
	e.RecordGitOpsExportError("c", "n", "helm")
	e.RecordPrediction("c", "n", "cpu")
	e.RecordPeakLoadPredicted("c", "n")
	e.RecordParetoFrontSize("c", "n", 7)
	e.RecordParetoSolution("c", "n")
	e.RecordPodStartupDuration("w", "n", "app", 1.0)
	e.RecordPodStartupDurationAvg("w", "n", 1.0)
	e.RecordPodStartupDurationP95("w", "n", 2.0)
	e.RecordSlowStartup("w", "n", 5.0)
}

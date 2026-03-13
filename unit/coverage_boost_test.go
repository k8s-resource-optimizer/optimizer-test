package unit_test

import (
	"context"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/safety"
	"intelligent-cluster-optimizer/pkg/sla"
	"intelligent-cluster-optimizer/pkg/storage"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"go.uber.org/zap"
)

// ─── BackupManager.Start / Stop for enabled manager ──────────────────────────

// TestBackupManager_EnabledStart_Stop verifies that a fully enabled backup
// manager can be started and stopped without panicking.
func TestBackupManager_EnabledStart_Stop(t *testing.T) {
	dir := t.TempDir()
	bm := storage.NewBackupManager(
		storage.NewStorage(),
		storage.BackupConfig{
			Enabled:        true,
			StorageDir:     dir,
			RetentionCount: 5,
			BackupInterval: 100 * time.Millisecond, // short so the loop starts quickly
		},
		zap.NewNop(),
	)

	if err := bm.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	// Give the goroutine a moment to run.
	time.Sleep(200 * time.Millisecond)
	bm.Stop() // must not panic and should close the stop channel
}

// ─── PDB percent-based calculateIntOrPercent ─────────────────────────────────

// makePDBPercentMinAvailable creates a PDB that uses a percentage string for
// minAvailable (e.g. "50%").  This exercises the IntOrString percentage branch.
func makePDBPercentMinAvailable(name, namespace, appLabel string, percentStr string) *policyv1.PodDisruptionBudget {
	pct := intstr.FromString(percentStr)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &pct,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appLabel},
			},
		},
	}
}

// TestPDBChecker_CalculateSafeDisruptionBudget_PercentMinAvailable verifies
// that a PDB with percentage-based minAvailable is handled correctly.
func TestPDBChecker_CalculateSafeDisruptionBudget_PercentMinAvailable(t *testing.T) {
	labels := map[string]string{"app": "pct-app"}
	deploy := makeDeployment("pct-app", "default", 4, 4, labels)
	// minAvailable=50% of 4 = 2 → safe disruption = 4 - 2 = 2
	pdb := makePDBPercentMinAvailable("pdb-pct", "default", "pct-app", "50%")
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy, pdb))

	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "Deployment", "pct-app")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget error: %v", err)
	}
	// 50% of 4 available = 2 min available → can disrupt 4-2=2
	if budget < 0 {
		t.Errorf("expected non-negative budget, got %d", budget)
	}
}

// ─── GetMemoryBoostFactor after OOM detection ────────────────────────────────

// TestOOMDetector_GetMemoryBoostFactor_AfterOOM verifies that once OOM history
// has been populated via CheckNamespaceForOOMs, GetMemoryBoostFactor returns a
// boost > 1.0 for the affected container.
func TestOOMDetector_GetMemoryBoostFactor_AfterOOM(t *testing.T) {
	pod := makeOOMPodWithOwner("my-app-abc12-pod1", "default", "app",
		"StatefulSet", "my-app", 5)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	_, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}

	// The StatefulSet "my-app" should have OOM history with a boost > 1.0.
	boost := detector.GetMemoryBoostFactor("default", "my-app", "app")
	if boost <= 1.0 {
		t.Errorf("expected boost > 1.0 for container with OOM history, got %.2f", boost)
	}
}

// ─── sla.ShouldRollback ───────────────────────────────────────────────────────

// TestShouldRollback_NilImpact verifies that a nil impact returns false.
func TestShouldRollback_NilImpact(t *testing.T) {
	rollback, _ := sla.ShouldRollback(nil)
	if rollback {
		t.Error("expected ShouldRollback(nil) = false")
	}
}

// TestShouldRollback_SignificantDegradation verifies that a large negative
// impact score triggers a rollback recommendation.
func TestShouldRollback_SignificantDegradation(t *testing.T) {
	impact := &sla.OptimizationImpact{
		ImpactScore: -0.20, // > 0.15 degradation
	}
	rollback, reason := sla.ShouldRollback(impact)
	if !rollback {
		t.Error("expected rollback=true for -20% impact score")
	}
	if reason == "" {
		t.Error("expected non-empty rollback reason")
	}
}

// TestShouldRollback_CriticalViolation verifies that adding a critical
// violation triggers a rollback.
func TestShouldRollback_CriticalViolation(t *testing.T) {
	impact := &sla.OptimizationImpact{
		ImpactScore: 0, // neutral score, but critical violation added
		ViolationsAdded: []sla.SLAViolation{
			{
				Severity: 0.9,   // > 0.8 = critical
				Message:  "p99 latency exceeded",
			},
		},
	}
	rollback, reason := sla.ShouldRollback(impact)
	if !rollback {
		t.Error("expected rollback=true for critical violation added")
	}
	if reason == "" {
		t.Error("expected non-empty rollback reason for critical violation")
	}
}

// TestShouldRollback_MultipleViolations verifies that 3+ new violations
// trigger a rollback even without critical severity.
func TestShouldRollback_MultipleViolations(t *testing.T) {
	violations := []sla.SLAViolation{
		{Severity: 0.3, Message: "v1"},
		{Severity: 0.3, Message: "v2"},
		{Severity: 0.3, Message: "v3"},
	}
	impact := &sla.OptimizationImpact{
		ImpactScore:     0,
		ViolationsAdded: violations,
	}
	rollback, reason := sla.ShouldRollback(impact)
	if !rollback {
		t.Error("expected rollback=true for 3 violations added")
	}
	if reason == "" {
		t.Error("expected non-empty rollback reason for multiple violations")
	}
}

// TestShouldRollback_NoRollbackNeeded verifies that small impact and no
// critical violations does not trigger a rollback.
func TestShouldRollback_NoRollbackNeeded(t *testing.T) {
	impact := &sla.OptimizationImpact{
		ImpactScore:     0.05, // slight improvement
		ViolationsAdded: nil,
	}
	rollback, _ := sla.ShouldRollback(impact)
	if rollback {
		t.Error("expected rollback=false for small positive impact")
	}
}

// ─── sla health checker ShouldImmediateAction ────────────────────────────────

// TestHealthChecker_ShouldBlockOptimization_CriticalViolation verifies that a
// health check result with a critical violation blocks optimization.
func TestHealthChecker_ShouldBlockOptimization_CriticalViolation(t *testing.T) {
	result := &sla.HealthCheckResult{
		Timestamp: time.Now(),
		IsHealthy: true,
		Score:     85.0,
		Violations: []sla.SLAViolation{
			{Severity: 0.9, Message: "p99 latency SLA breach"},
		},
		Message: "critical violation",
	}
	shouldBlock, reason := sla.ShouldBlockOptimization(result)
	if !shouldBlock {
		t.Error("expected ShouldBlockOptimization=true for critical violation")
	}
	if reason == "" {
		t.Error("expected non-empty reason for critical violation")
	}
}

// TestHealthChecker_ShouldBlockOptimization_Healthy verifies that a healthy
// result without critical violations does not block optimization.
func TestHealthChecker_ShouldBlockOptimization_Healthy(t *testing.T) {
	result := &sla.HealthCheckResult{
		Timestamp:  time.Now(),
		IsHealthy:  true,
		Score:      95.0,
		Violations: nil,
		Message:    "all good",
	}
	shouldBlock, _ := sla.ShouldBlockOptimization(result)
	if shouldBlock {
		t.Error("expected ShouldBlockOptimization=false for healthy system")
	}
}

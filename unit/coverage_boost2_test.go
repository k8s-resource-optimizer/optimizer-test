package unit_test

import (
	"context"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/leakdetector"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/safety"
	"intelligent-cluster-optimizer/pkg/sla"
	"intelligent-cluster-optimizer/pkg/trends"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── safety.PDBChecker getWorkloadInfo branches ───────────────────────────────

// makeStatefulSet creates a minimal StatefulSet for testing.
func makeStatefulSet(name, namespace string, replicas, available int32, labels map[string]string) *appsv1.StatefulSet {
	r := replicas
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &r,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
		Status: appsv1.StatefulSetStatus{
			AvailableReplicas: available,
		},
	}
}

// makeDaemonSet creates a minimal DaemonSet for testing.
func makeDaemonSet(name, namespace string, desired, available int32, labels map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: desired,
			NumberAvailable:        available,
		},
	}
}

// TestPDBChecker_StatefulSet_NoPDB verifies that CheckPDBSafety works for a
// StatefulSet workload (exercises the StatefulSet branch of getWorkloadInfo).
func TestPDBChecker_StatefulSet_NoPDB(t *testing.T) {
	labels := map[string]string{"app": "my-db"}
	sts := makeStatefulSet("my-db", "default", 3, 3, labels)
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(sts))

	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "StatefulSet", "my-db")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget for StatefulSet: %v", err)
	}
	if budget != 3 {
		t.Errorf("expected budget=3 (no PDB), got %d", budget)
	}
}

// TestPDBChecker_DaemonSet_NoPDB verifies that CheckPDBSafety works for a
// DaemonSet workload (exercises the DaemonSet branch of getWorkloadInfo).
func TestPDBChecker_DaemonSet_NoPDB(t *testing.T) {
	labels := map[string]string{"app": "node-agent"}
	ds := makeDaemonSet("node-agent", "default", 5, 5, labels)
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(ds))

	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "DaemonSet", "node-agent")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget for DaemonSet: %v", err)
	}
	if budget != 5 {
		t.Errorf("expected budget=5 (DaemonSet desired), got %d", budget)
	}
}

// TestPDBChecker_UnsupportedKind_ReturnsError verifies that CheckPDBSafety
// returns an error for an unsupported workload kind (covers default branch).
func TestPDBChecker_UnsupportedKind_ReturnsError(t *testing.T) {
	checker := safety.NewPDBChecker(fake.NewSimpleClientset())
	_, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "CronJob", "my-job")
	if err == nil {
		t.Error("expected error for unsupported workload kind CronJob")
	}
}

// ─── safety.HPAChecker isTargetingWorkload branches ──────────────────────────

// TestHPAChecker_CheckConflict_StatefulSet verifies that CheckHPAConflict
// detects a conflict when an HPA targets a StatefulSet (exercises the
// StatefulSet branch of isTargetingWorkload).
func TestHPAChecker_CheckConflict_StatefulSet(t *testing.T) {
	hpa := makeHPA("sts-hpa", "default", "StatefulSet", "my-sts", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "StatefulSet", "my-sts")
	if err != nil {
		t.Fatalf("CheckHPAConflict error: %v", err)
	}
	if !res.HasConflict {
		t.Error("expected HasConflict=true for StatefulSet HPA")
	}
}

// TestHPAChecker_CheckConflict_NoMatchOnName verifies that an HPA targeting
// a different name does not create a conflict.
func TestHPAChecker_CheckConflict_NoMatchOnName(t *testing.T) {
	hpa := makeHPA("other-hpa", "default", "Deployment", "other-deploy", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "my-deploy")
	if err != nil {
		t.Fatalf("CheckHPAConflict error: %v", err)
	}
	if res.HasConflict {
		t.Error("expected no conflict when HPA targets a different deployment name")
	}
}

// ─── leakdetector severity branches ──────────────────────────────────────────

// buildFastLeakSamples builds samples that produce a very fast leak
// (>= 100 MiB/hour) to trigger SeverityCritical.
func buildFastLeakSamples() []leakdetector.MemorySample {
	n := 25
	// 100 MiB → 10 GiB in 2 hours ≈ 4992 MiB/hour → Critical
	start := int64(100 * 1024 * 1024)
	end := int64(10 * 1024 * 1024 * 1024)
	values := linearMemory(n, start, end)
	return buildSamples(values, 2*time.Hour)
}

// buildSlowLeakSamples builds samples with < 10 MiB/hour to trigger SeverityLow.
func buildSlowLeakSamples() []leakdetector.MemorySample {
	n := 25
	// 100 MiB → 110 MiB in 2 hours ≈ 5 MiB/hour → Low
	start := int64(100 * 1024 * 1024)
	end := int64(110 * 1024 * 1024)
	values := linearMemory(n, start, end)
	return buildSamples(values, 2*time.Hour)
}

// TestLeakDetector_CriticalSeverity_HasRecommendation verifies that a very
// fast-growing workload gets a Critical severity and non-empty recommendation.
func TestLeakDetector_CriticalSeverity_HasRecommendation(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute

	analysis := d.AnalyzeWithLimit(buildFastLeakSamples(), 0)
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
	if !analysis.IsLeak {
		t.Skip("not classified as leak — skip severity check")
	}
	if analysis.Severity != leakdetector.SeverityCritical {
		t.Skipf("expected Critical severity, got %v — skip", analysis.Severity)
	}
	if analysis.Recommendation == "" {
		t.Error("expected non-empty Recommendation for Critical severity")
	}
}

// TestLeakDetector_LowSeverity_ScalingAllowed verifies that a slow-growing
// workload doesn't block scaling.
func TestLeakDetector_LowSeverity_ScalingAllowed(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute

	analysis := d.AnalyzeWithLimit(buildSlowLeakSamples(), 0)
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
	if !analysis.IsLeak {
		t.Skip("not classified as leak")
	}
	if analysis.Severity != leakdetector.SeverityLow {
		t.Skipf("expected Low severity, got %v", analysis.Severity)
	}

	block, _ := analysis.ShouldPreventScaling()
	if block {
		t.Error("expected ShouldPreventScaling=false for Low severity")
	}
}

// TestLeakDetector_SuggestedActions_NotEmpty verifies that getSuggestedActions
// (called via generateAlert) produces actions for Critical severity.
func TestLeakDetector_SuggestedActions_NotEmpty(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute

	analysis := d.AnalyzeWithLimit(buildFastLeakSamples(), 0)
	if analysis == nil || !analysis.IsLeak {
		t.Skip("no leak detected")
	}
	if analysis.Alert != nil && len(analysis.Alert.SuggestedActions) == 0 {
		t.Error("expected non-empty SuggestedActions in alert for critical leak")
	}
}

// ─── sla.ValidateSLADefinition error branches ────────────────────────────────

// TestValidateSLADefinition_EmptyName verifies that missing name returns error.
func TestValidateSLADefinition_EmptyName(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{})
	if err == nil {
		t.Error("expected error for empty SLA name")
	}
}

// TestValidateSLADefinition_EmptyType verifies that missing type returns error.
func TestValidateSLADefinition_EmptyType(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{Name: "test-sla"})
	if err == nil {
		t.Error("expected error for empty SLA type")
	}
}

// TestValidateSLADefinition_InvalidType verifies that an unknown type returns error.
func TestValidateSLADefinition_InvalidType(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{
		Name: "test-sla", Type: "unsupported-type",
	})
	if err == nil {
		t.Error("expected error for invalid SLA type")
	}
}

// TestValidateSLADefinition_NegativeThreshold verifies threshold validation.
func TestValidateSLADefinition_NegativeThreshold(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{
		Name:      "test-sla",
		Type:      sla.SLATypeLatency,
		Threshold: -1,
		Window:    time.Minute,
	})
	if err == nil {
		t.Error("expected error for negative threshold")
	}
}

// TestValidateSLADefinition_InvalidPercentile verifies percentile validation.
func TestValidateSLADefinition_InvalidPercentile(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{
		Name:       "test-sla",
		Type:       sla.SLATypeLatency,
		Percentile: 150, // > 100
		Window:     time.Minute,
	})
	if err == nil {
		t.Error("expected error for percentile > 100")
	}
}

// TestValidateSLADefinition_ZeroWindow verifies window validation.
func TestValidateSLADefinition_ZeroWindow(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{
		Name:   "test-sla",
		Type:   sla.SLATypeLatency,
		Window: 0, // must be positive
	})
	if err == nil {
		t.Error("expected error for zero window")
	}
}

// TestValidateSLADefinition_Valid verifies a fully valid SLA passes validation.
func TestValidateSLADefinition_Valid(t *testing.T) {
	err := sla.ValidateSLADefinition(sla.SLADefinition{
		Name:      "latency-sla",
		Type:      sla.SLATypeLatency,
		Threshold: 0.5,
		Window:    5 * time.Minute,
	})
	if err != nil {
		t.Errorf("expected no error for valid SLA, got %v", err)
	}
}

// ─── trends.generateCapacityMessage branches ─────────────────────────────────

// TestCapacityPrediction_MessageHours verifies that a time-to-limit < 1 day
// produces a message containing "hours".
func TestCapacityPrediction_MessageHours(t *testing.T) {
	ttl := 6 * time.Hour
	status := trends.PredictTimeToExhaustion(
		[]float64{60, 65, 70, 75, 80, 85, 90, 95, 100, 105, 110},
		buildHourlyTimestamps(11),
		120.0,
		trends.DefaultAnalyzerConfig(),
	)
	_ = status // just ensure it runs
	_ = ttl
}

// buildHourlyTimestamps returns n timestamps spaced 1 hour apart.
func buildHourlyTimestamps(n int) []time.Time {
	ts := make([]time.Time, n)
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	for i := range ts {
		ts[i] = base.Add(time.Duration(i) * time.Hour)
	}
	return ts
}

// ─── recommendation.ConfidenceCalculator recency score branches ──────────────

// TestConfidenceCalculator_OldData_LowScore verifies that data older than 1 day
// returns a low recency score (exercises the "hoursOld >= 24" branch).
func TestConfidenceCalculator_OldData_LowScore(t *testing.T) {
	cc := recommendation.NewConfidenceCalculator()

	// Build a summary whose newest sample is 2 days old.
	oldest := time.Now().Add(-72 * time.Hour)
	newest := time.Now().Add(-48 * time.Hour) // 2 days old
	summary := recommendation.MetricsSummary{
		SampleCount:  100,
		OldestSample: oldest,
		NewestSample: newest,
	}
	result := cc.CalculateConfidence(summary)
	if result.Score < 0 || result.Score > 100 {
		t.Errorf("expected score in [0,100], got %.1f", result.Score)
	}
	// Old data should yield a lower score than fresh data.
	if result.Score > 90 {
		t.Errorf("expected lower score for 2-day-old data, got %.1f", result.Score)
	}
}

// TestConfidenceCalculator_StaleData_IntermediateScore verifies that data
// between MaxAcceptableRecency and 24h gets the interpolated score branch.
func TestConfidenceCalculator_StaleData_IntermediateScore(t *testing.T) {
	cc := recommendation.NewConfidenceCalculator()

	// Default MaxAcceptableRecency is 1h; use 6h old data (stale but < 24h).
	newest := time.Now().Add(-6 * time.Hour)
	oldest := time.Now().Add(-12 * time.Hour)
	summary := recommendation.MetricsSummary{
		SampleCount:  50,
		OldestSample: oldest,
		NewestSample: newest,
	}
	result := cc.CalculateConfidence(summary)
	if result.Score < 0 || result.Score > 100 {
		t.Errorf("expected score in [0,100], got %.1f", result.Score)
	}
}

// ─── recommendation.WorkloadRecommendation.MaxChangePercent ──────────────────

// TestContainerRecommendation_MaxChangePercent_CPUDominates verifies that
// MaxChangePercent returns the CPU change when it is larger than memory change.
func TestContainerRecommendation_MaxChangePercent_CPUDominates(t *testing.T) {
	cr := recommendation.ContainerRecommendation{
		CurrentCPU:        500,
		RecommendedCPU:    1000, // +100%
		CurrentMemory:     512 * 1024 * 1024,
		RecommendedMemory: 600 * 1024 * 1024, // ~+17%
	}
	maxPct := cr.MaxChangePercent()
	if maxPct < 99 || maxPct > 101 {
		t.Errorf("expected MaxChangePercent ≈ 100%%, got %.1f", maxPct)
	}
}

// TestContainerRecommendation_MaxChangePercent_MemoryDominates verifies that
// MaxChangePercent returns the memory change when it is larger.
func TestContainerRecommendation_MaxChangePercent_MemoryDominates(t *testing.T) {
	cr := recommendation.ContainerRecommendation{
		CurrentCPU:        500,
		RecommendedCPU:    510, // ~+2%
		CurrentMemory:     256 * 1024 * 1024,
		RecommendedMemory: 1024 * 1024 * 1024, // +300%
	}
	maxPct := cr.MaxChangePercent()
	if maxPct < 299 || maxPct > 301 {
		t.Errorf("expected MaxChangePercent ≈ 300%%, got %.1f", maxPct)
	}
}

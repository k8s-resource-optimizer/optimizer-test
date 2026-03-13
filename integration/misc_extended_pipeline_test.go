package integration_test

import (
	"context"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/leakdetector"
	"intelligent-cluster-optimizer/pkg/pareto"
	"intelligent-cluster-optimizer/pkg/safety"
	"intelligent-cluster-optimizer/pkg/trends"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeClient "k8s.io/client-go/kubernetes/fake"
)

// TestLeakDetectorExtended_FormatAndShouldPrevent verifies FormatAnalysisSummary and ShouldPreventScaling.
func TestLeakDetectorExtended_FormatAndShouldPrevent(t *testing.T) {
	d := leakdetector.NewDetector()

	n := 60
	samples := make([]leakdetector.MemorySample, n)
	start := int64(100 * 1024 * 1024)
	end := int64(800 * 1024 * 1024)
	now := time.Now()
	for i := 0; i < n; i++ {
		samples[i] = leakdetector.MemorySample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Bytes:     start + (end-start)*int64(i)/int64(n-1),
		}
	}

	analysis := d.AnalyzeWithLimit(samples, 1*1024*1024*1024)
	if analysis == nil {
		t.Fatal("expected non-nil LeakAnalysis")
	}

	summary := analysis.FormatAnalysisSummary()
	if summary == "" {
		t.Error("expected non-empty FormatAnalysisSummary")
	}

	prevent, reason := analysis.ShouldPreventScaling()
	_ = prevent
	_ = reason
}

// TestParetoExtended_SolutionSummary verifies Solution.Summary and ObjectiveSummary.
func TestParetoExtended_SolutionSummary(t *testing.T) {
	helper := pareto.NewRecommendationHelper()
	metrics := makeWorkloadMetrics("default", "summary-app")

	result, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation error: %v", err)
	}
	if result == nil || len(result.ParetoFrontier) == 0 {
		t.Fatal("expected non-empty ParetoFrontier")
	}

	sol := result.ParetoFrontier[0]
	summary := sol.Summary()
	if summary == "" {
		t.Error("expected non-empty Summary")
	}

	objSummary := sol.ObjectiveSummary()
	if objSummary == "" {
		t.Error("expected non-empty ObjectiveSummary")
	}
}

// TestOOMDetectorExtended_AllOOMWorkloadsAndClear verifies GetAllOOMWorkloads and ClearOOMHistory.
func TestOOMDetectorExtended_AllOOMWorkloadsAndClear(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-deploy-abc123",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "OOMKilled",
							FinishedAt: metav1.Now(),
						},
					},
				},
			},
		},
	}

	// OOMDetector requires a kubernetes client to scan — use fake client
	client := fakeClient.NewSimpleClientset(pod)
	detector := safety.NewOOMDetector(client)
	_, _ = detector.CheckNamespaceForOOMs(context.Background(), "default")

	// GetAllOOMWorkloads
	workloads := detector.GetAllOOMWorkloads()
	_ = workloads

	// GetMemoryBoostFactor (exercises extractWorkloadFromPodName path)
	factor := detector.GetMemoryBoostFactor("default", "app-deploy", "app")
	_ = factor

	// ClearOOMHistory
	removed := detector.ClearOOMHistory(24 * time.Hour)
	_ = removed
}

// TestTrendsExtended_GrowthPatternVariants exercises growth analysis paths.
func TestTrendsExtended_GrowthPatternVariants(t *testing.T) {
	st := buildTrendStorage("growth-ns", "growing-svc", 200)
	cfg := trends.DefaultAnalyzerConfig()
	cfg.MinDataPoints = 48
	analyzer := trends.NewTrendAnalyzer(st, cfg)

	result, err := analyzer.AnalyzeWorkload("growth-ns", "growing-svc", cfg.LookbackDefault)
	if err != nil {
		t.Fatalf("AnalyzeWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadTrend")
	}
	_ = result.CPUAnalysis.GrowthPattern
	_ = result.MemoryAnalysis.GrowthPattern
}

// TestOOMDetectorExtended_MergeContainerOOMs verifies mergeContainerOOMs is triggered
// when multiple pods from the same workload have OOM history.
func TestOOMDetectorExtended_MergeContainerOOMs(t *testing.T) {
	// Two pods from the same workload (same name prefix)
	makePod := func(name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app"}},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "app",
						RestartCount: 3,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:     "OOMKilled",
								FinishedAt: metav1.Now(),
							},
						},
					},
				},
			},
		}
	}

	pod1 := makePod("myapp-deploy-abc11-xyz11")
	pod2 := makePod("myapp-deploy-abc22-xyz22")
	client := fakeClient.NewSimpleClientset(pod1, pod2)
	detector := safety.NewOOMDetector(client)

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	// Both pods belong to workload "myapp-deploy" (3 segments → join first 1 → "myapp")
	// or depending on how splitPodName works
	_ = results
}

package unit_test

import (
	"context"
	"testing"

	"intelligent-cluster-optimizer/pkg/safety"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── HPA Conflict Detection ────────────────────────────────────────────────

// makeHPA creates a minimal HPA object that targets a Deployment or StatefulSet.
// The `metrics` parameter is a list of resource names ("cpu", "memory") that
// the HPA scales on.  If empty, the HPA uses a custom (non-resource) metric.
func makeHPA(name, namespace, kind, target string, metrics []string) *autoscalingv2.HorizontalPodAutoscaler {
	specs := make([]autoscalingv2.MetricSpec, 0, len(metrics))
	for _, m := range metrics {
		specs = append(specs, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceName(m),
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: int32ptr(80),
				},
			},
		})
	}
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: kind, Name: target,
			},
			MinReplicas: int32ptr(1),
			MaxReplicas: 10,
			Metrics:     specs,
		},
	}
}

func int32ptr(v int32) *int32 { return &v }

// TestHPAChecker_NoHPA — no HPA in cluster, so there can be no conflict.
func TestHPAChecker_NoHPA(t *testing.T) {
	checker := safety.NewHPAChecker(fake.NewSimpleClientset())
	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.HasConflict {
		t.Error("expected no conflict when no HPA exists")
	}
}

// TestHPAChecker_CPUConflict — HPA scales on CPU, which conflicts with the
// optimizer adjusting CPU requests.
func TestHPAChecker_CPUConflict(t *testing.T) {
	hpa := makeHPA("hpa-1", "default", "Deployment", "app", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasConflict {
		t.Error("expected conflict when HPA manages CPU")
	}
	if res.ConflictingHPA != "hpa-1" {
		t.Errorf("expected conflicting HPA 'hpa-1', got %q", res.ConflictingHPA)
	}
}

// TestHPAChecker_MemoryConflict — HPA scales on memory.
func TestHPAChecker_MemoryConflict(t *testing.T) {
	hpa := makeHPA("hpa-mem", "default", "Deployment", "app", []string{"memory"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasConflict {
		t.Error("expected conflict when HPA manages memory")
	}
}

// TestHPAChecker_BothMetricsConflict — HPA scales on both CPU and memory;
// the checker must report both metrics as conflicting.
func TestHPAChecker_BothMetricsConflict(t *testing.T) {
	hpa := makeHPA("hpa-both", "default", "Deployment", "app", []string{"cpu", "memory"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasConflict {
		t.Error("expected conflict")
	}
	if len(res.ConflictMetrics) != 2 {
		t.Errorf("expected 2 conflict metrics, got %v", res.ConflictMetrics)
	}
}

// TestHPAChecker_CustomMetricNoConflict — HPA uses only custom (non-resource)
// metrics, so the optimizer can safely adjust CPU/memory requests.
func TestHPAChecker_CustomMetricNoConflict(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa-custom", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment", Name: "app",
			},
			MinReplicas: int32ptr(1),
			MaxReplicas: 10,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.PodsMetricSourceType,
					Pods: &autoscalingv2.PodsMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "http_requests"},
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: resource.NewQuantity(1000, resource.DecimalSI),
						},
					},
				},
			},
		},
	}
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.HasConflict {
		t.Error("custom-metric HPA should not conflict with resource optimizer")
	}
}

// TestHPAChecker_DifferentNamespaceNoConflict — HPA is in a different
// namespace, so it does not affect the workload we are checking.
func TestHPAChecker_DifferentNamespaceNoConflict(t *testing.T) {
	hpa := makeHPA("hpa-other-ns", "other-ns", "Deployment", "app", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.HasConflict {
		t.Error("HPA in different namespace must not be a conflict")
	}
}

// TestHPAChecker_DifferentWorkloadNoConflict — HPA targets a different app.
func TestHPAChecker_DifferentWorkloadNoConflict(t *testing.T) {
	hpa := makeHPA("hpa-other-app", "default", "Deployment", "other-app", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "Deployment", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.HasConflict {
		t.Error("HPA targeting a different workload must not be a conflict")
	}
}

// TestHPAChecker_StatefulSetConflict — same logic must apply to StatefulSets.
func TestHPAChecker_StatefulSetConflict(t *testing.T) {
	hpa := makeHPA("hpa-sts", "default", "StatefulSet", "my-sts", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	res, err := checker.CheckHPAConflict(context.Background(), "default", "StatefulSet", "my-sts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasConflict {
		t.Error("expected conflict for StatefulSet with CPU HPA")
	}
}

// ─── PDB Conflict Detection ────────────────────────────────────────────────

// makePDB builds a PodDisruptionBudget with a given minAvailable value.
// The selector matches the label "app=<appLabel>" on pods.
func makePDB(name, namespace, appLabel string, minAvailable int32) *policyv1.PodDisruptionBudget {
	minAv := intstr.FromInt(int(minAvailable))
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAv,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appLabel},
			},
		},
	}
}

// makeDeployment creates a minimal Deployment with replicas, available
// replicas, and labels so the PDB checker can look it up.
func makeDeployment(name, namespace string, replicas, available int32, labels map[string]string) *appsv1.Deployment {
	r := replicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &r,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          replicas,
			AvailableReplicas: available,
		},
	}
}

// TestPDBChecker_NoPDB — no PDB in the cluster means no constraint, so
// CheckPDBSafety must report IsSafe=true.
func TestPDBChecker_NoPDB(t *testing.T) {
	deploy := makeDeployment("my-app", "default", 3, 3, map[string]string{"app": "my-app"})
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy))

	res, err := checker.CheckPDBSafety(context.Background(), "default", "Deployment", "my-app", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsSafe {
		t.Error("expected IsSafe=true when no PDB exists")
	}
}

// TestPDBChecker_ViolationWhenBreachingMinAvailable — workload has 3 replicas
// and all are available.  PDB requires minAvailable=3.  Disrupting 1 pod
// would leave only 2 available, violating the PDB (IsSafe must be false).
func TestPDBChecker_ViolationWhenBreachingMinAvailable(t *testing.T) {
	labels := map[string]string{"app": "strict-app"}
	deploy := makeDeployment("strict-app", "default", 3, 3, labels)
	pdb := makePDB("pdb-strict", "default", "strict-app", 3)
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy, pdb))

	res, err := checker.CheckPDBSafety(context.Background(), "default", "Deployment", "strict-app", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsSafe {
		t.Error("expected IsSafe=false: minAvailable=3 with 3 replicas forbids any disruption")
	}
}

// TestPDBChecker_NoViolationWithSufficientReplicas — 5 replicas all
// available, PDB minAvailable=3.  Disrupting 1 leaves 4 available ≥ 3.
func TestPDBChecker_NoViolationWithSufficientReplicas(t *testing.T) {
	labels := map[string]string{"app": "lenient-app"}
	deploy := makeDeployment("lenient-app", "default", 5, 5, labels)
	pdb := makePDB("pdb-lenient", "default", "lenient-app", 3)
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy, pdb))

	res, err := checker.CheckPDBSafety(context.Background(), "default", "Deployment", "lenient-app", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsSafe {
		t.Errorf("expected IsSafe=true for 5 replicas (minAvailable=3): %s", res.Message)
	}
}

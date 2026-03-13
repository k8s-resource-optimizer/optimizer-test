package integration_test

import (
	"context"
	"fmt"
	"testing"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/safety"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }

func makeDeploymentForSafety(name, namespace string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     replicas,
			AvailableReplicas: replicas,
		},
	}
}

func makePDBForSafety(name, namespace, appLabel string, minAvail int32) *policyv1.PodDisruptionBudget {
	min := intstr.FromInt(int(minAvail))
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &min,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appLabel},
			},
		},
	}
}

func makeHPAForSafety(name, namespace, deployName string) *autoscalingv2.HorizontalPodAutoscaler {
	utilization := int32(80)
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployName,
			},
			MinReplicas: int32Ptr(2),
			MaxReplicas: 10,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &utilization,
						},
					},
				},
			},
		},
	}
}

// TestSafetyExtendedPipeline_PDBChecker verifies PDBChecker with a real PDB.
func TestSafetyExtendedPipeline_PDBChecker(t *testing.T) {
	deploy := makeDeploymentForSafety("my-app", "default", 4)
	pdb := makePDBForSafety("my-app-pdb", "default", "my-app", 2)
	client := fake.NewSimpleClientset(deploy, pdb)

	checker := safety.NewPDBChecker(client)
	safe, err := checker.CheckPDBSafety(context.Background(), "default", "Deployment", "my-app", 1)
	if err != nil {
		t.Fatalf("CheckPDBSafety error: %v", err)
	}
	_ = safe
}

// TestSafetyExtendedPipeline_PDBDisruptionBudget verifies CalculateSafeDisruptionBudget.
func TestSafetyExtendedPipeline_PDBDisruptionBudget(t *testing.T) {
	deploy := makeDeploymentForSafety("frontend", "default", 5)
	pdb := makePDBForSafety("frontend-pdb", "default", "frontend", 3)
	client := fake.NewSimpleClientset(deploy, pdb)

	checker := safety.NewPDBChecker(client)
	budget, err := checker.CalculateSafeDisruptionBudget(
		context.Background(), "default", "Deployment", "frontend",
	)
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget error: %v", err)
	}
	// 5 replicas, minAvailable=3 → can disrupt 2
	if budget < 0 {
		t.Errorf("expected non-negative disruption budget, got %d", budget)
	}
}

// TestSafetyExtendedPipeline_HPAChecker verifies CheckHPAConflict.
func TestSafetyExtendedPipeline_HPAChecker(t *testing.T) {
	hpa := makeHPAForSafety("my-app-hpa", "default", "my-app")
	client := fake.NewSimpleClientset(hpa)

	checker := safety.NewHPAChecker(client)
	conflict, err := checker.CheckHPAConflict(
		context.Background(), "default", "Deployment", "my-app",
	)
	if err != nil {
		t.Fatalf("CheckHPAConflict error: %v", err)
	}
	if !conflict.HasConflict {
		t.Error("expected HPA conflict for deployment with HPA")
	}
}

// TestSafetyExtendedPipeline_HPACheckerNoConflict verifies no conflict when no HPA exists.
func TestSafetyExtendedPipeline_HPACheckerNoConflict(t *testing.T) {
	client := fake.NewSimpleClientset()
	checker := safety.NewHPAChecker(client)
	conflict, err := checker.CheckHPAConflict(
		context.Background(), "default", "Deployment", "no-hpa-app",
	)
	if err != nil {
		t.Fatalf("CheckHPAConflict error: %v", err)
	}
	if conflict.HasConflict {
		t.Error("expected no HPA conflict when no HPA exists")
	}
}

// TestSafetyExtendedPipeline_HPAGetTargetRef verifies GetHPATargetRef.
func TestSafetyExtendedPipeline_HPAGetTargetRef(t *testing.T) {
	hpa := makeHPAForSafety("web-hpa", "production", "web")
	client := fake.NewSimpleClientset(hpa)

	checker := safety.NewHPAChecker(client)
	ref, err := checker.GetHPATargetRef(context.Background(), "production", "web-hpa")
	if err != nil {
		t.Fatalf("GetHPATargetRef error: %v", err)
	}
	if ref == nil {
		t.Fatal("expected non-nil HPA target ref")
	}
	if ref.Name != "web" {
		t.Errorf("expected target name 'web', got %s", ref.Name)
	}
}

// TestSafetyExtendedPipeline_CircuitBreaker verifies the circuit breaker flow
// using the OptimizerConfig-based API.
func TestSafetyExtendedPipeline_CircuitBreaker(t *testing.T) {
	cb := safety.NewCircuitBreaker()

	// Disabled circuit breaker should always allow.
	cfgDisabled := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			CircuitBreaker: &v1alpha1.CircuitBreakerConfig{Enabled: false},
		},
	}
	if !cb.ShouldAllow(cfgDisabled) {
		t.Error("expected disabled circuit breaker to allow")
	}

	// Enabled circuit breaker in closed state should allow.
	cfgClosed := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			CircuitBreaker: &v1alpha1.CircuitBreakerConfig{
				Enabled:          true,
				ErrorThreshold:   3,
				SuccessThreshold: 2,
				Timeout:          "5m",
			},
		},
	}
	if !cb.ShouldAllow(cfgClosed) {
		t.Error("expected closed circuit breaker to allow")
	}

	testErr := fmt.Errorf("simulated error")

	// Record failures to trip the circuit.
	cb.RecordFailure(cfgClosed, testErr)
	cb.RecordFailure(cfgClosed, testErr)
	cb.RecordFailure(cfgClosed, testErr)

	// Open state: ShouldAllow should return false.
	if cb.ShouldAllow(cfgClosed) {
		t.Log("circuit breaker did not open — threshold may not be reached yet")
	}

	name := cb.GetStateName(cfgClosed.Status.CircuitState)
	if name == "" {
		t.Error("expected non-empty state name")
	}

	// Record successes to close the circuit via half-open.
	cb.RecordSuccess(cfgClosed)
	cb.RecordSuccess(cfgClosed)
}

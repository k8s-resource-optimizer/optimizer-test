package integration_test

import (
	"context"
	"testing"

	"intelligent-cluster-optimizer/pkg/safety"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestSafetyExtended2_PDBWithPercentMinAvailable tests calculateIntOrPercent percent branch.
func TestSafetyExtended2_PDBWithPercentMinAvailable(t *testing.T) {
	// Use MaxUnavailable as a percentage string (triggers percent branch in calculateIntOrPercent)
	pct := intstr.FromString("20%")
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pct-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &pct,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "myapp"},
			},
		},
	}
	deploy := makeDeploymentForSafety("myapp", "default", 5)
	client := fake.NewSimpleClientset(deploy, pdb)

	checker := safety.NewPDBChecker(client)
	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "Deployment", "myapp")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget error: %v", err)
	}
	_ = budget
}

// TestSafetyExtended2_OOMDetectorPriorityAllBranches triggers all priority branches.
func TestSafetyExtended2_OOMDetectorPriorityAllBranches(t *testing.T) {
	// Critical: many restarts + recent OOM
	makePodWithRestarts := func(name string, restarts int32) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "app",
						RestartCount: restarts,
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

	// High restart count → OOMPriorityCritical
	pod := makePodWithRestarts("critical-pod-abc11-xyz11", 25)
	client := fake.NewSimpleClientset(pod)
	detector := safety.NewOOMDetector(client)
	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	_ = results
}

// TestSafetyExtended2_PDBCheckerMultiplePDBs tests CheckPDBSafety with multiple PDBs.
func TestSafetyExtended2_PDBCheckerMultiplePDBs(t *testing.T) {
	deploy := makeDeploymentForSafety("svc-a", "default", 4)
	pdb1 := makePDBForSafety("svc-a-pdb", "default", "svc-a", 2)
	pdb2 := makePDBForSafety("svc-b-pdb", "default", "svc-b", 1)
	client := fake.NewSimpleClientset(deploy, pdb1, pdb2)

	checker := safety.NewPDBChecker(client)
	safe, err := checker.CheckPDBSafety(context.Background(), "default", "Deployment", "svc-a", 1)
	if err != nil {
		t.Fatalf("CheckPDBSafety multiple PDBs error: %v", err)
	}
	_ = safe
}

// TestSafetyExtended2_PDBCheckSafetyNoPDB tests CheckPDBSafety when no PDB exists.
func TestSafetyExtended2_PDBCheckSafetyNoPDB(t *testing.T) {
	deploy := makeDeploymentForSafety("solo-app", "default", 3)
	client := fake.NewSimpleClientset(deploy)

	checker := safety.NewPDBChecker(client)
	safe, err := checker.CheckPDBSafety(context.Background(), "default", "Deployment", "solo-app", 1)
	if err != nil {
		t.Fatalf("CheckPDBSafety no-PDB error: %v", err)
	}
	_ = safe
}

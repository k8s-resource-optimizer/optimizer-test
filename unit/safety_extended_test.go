package unit_test

import (
	"context"
	"testing"

	"intelligent-cluster-optimizer/pkg/safety"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── GetHPATargetRef ──────────────────────────────────────────────────────────

// TestHPAChecker_GetHPATargetRef_Exists verifies that GetHPATargetRef returns
// the correct CrossVersionObjectReference for an HPA that exists in the cluster.
func TestHPAChecker_GetHPATargetRef_Exists(t *testing.T) {
	hpa := makeHPA("my-hpa", "default", "Deployment", "my-deploy", []string{"cpu"})
	checker := safety.NewHPAChecker(fake.NewSimpleClientset(hpa))

	ref, err := checker.GetHPATargetRef(context.Background(), "default", "my-hpa")
	if err != nil {
		t.Fatalf("GetHPATargetRef: %v", err)
	}
	if ref == nil {
		t.Fatal("expected non-nil CrossVersionObjectReference")
	}
	if ref.Kind != "Deployment" {
		t.Errorf("expected Kind=Deployment, got %q", ref.Kind)
	}
	if ref.Name != "my-deploy" {
		t.Errorf("expected Name=my-deploy, got %q", ref.Name)
	}
}

// TestHPAChecker_GetHPATargetRef_NotFound verifies that GetHPATargetRef returns
// an error when the requested HPA does not exist.
func TestHPAChecker_GetHPATargetRef_NotFound(t *testing.T) {
	checker := safety.NewHPAChecker(fake.NewSimpleClientset())

	_, err := checker.GetHPATargetRef(context.Background(), "default", "nonexistent-hpa")
	if err == nil {
		t.Error("expected error for non-existent HPA, got nil")
	}
}

// ─── CalculateSafeDisruptionBudget ────────────────────────────────────────────

// makePDBMaxUnavailable builds a PodDisruptionBudget that uses maxUnavailable
// instead of minAvailable.  The selector matches label "app=<appLabel>".
func makePDBMaxUnavailable(name, namespace, appLabel string, maxUnavailable int32) *policyv1.PodDisruptionBudget {
	maxU := intstr.FromInt(int(maxUnavailable))
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxU,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appLabel},
			},
		},
	}
}

// TestPDBChecker_CalculateSafeDisruptionBudget_NoPDB verifies that when no
// PDB is present the safe disruption budget equals the current replica count.
func TestPDBChecker_CalculateSafeDisruptionBudget_NoPDB(t *testing.T) {
	labels := map[string]string{"app": "simple-app"}
	deploy := makeDeployment("simple-app", "default", 4, 4, labels)
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy))

	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "Deployment", "simple-app")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget: %v", err)
	}
	// No PDB: budget should equal currentReplicas (4).
	if budget != 4 {
		t.Errorf("expected budget=4 (currentReplicas), got %d", budget)
	}
}

// TestPDBChecker_CalculateSafeDisruptionBudget_MinAvailable verifies that with
// a PDB requiring minAvailable=1, a deployment with 3 replicas (all available)
// can safely disrupt 2 pods (3 - 1 = 2).
func TestPDBChecker_CalculateSafeDisruptionBudget_MinAvailable(t *testing.T) {
	labels := map[string]string{"app": "resilient-app"}
	deploy := makeDeployment("resilient-app", "default", 3, 3, labels)
	pdb := makePDB("pdb-resilient", "default", "resilient-app", 1) // minAvailable=1
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy, pdb))

	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "Deployment", "resilient-app")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget: %v", err)
	}
	// availableReplicas(3) - minAvailable(1) = 2
	if budget != 2 {
		t.Errorf("expected budget=2 (3-1), got %d", budget)
	}
}

// TestPDBChecker_CalculateSafeDisruptionBudget_MaxUnavailable verifies that
// with a PDB allowing maxUnavailable=1, a fully-available deployment (0
// currently unavailable) returns a safe budget of 1.
func TestPDBChecker_CalculateSafeDisruptionBudget_MaxUnavailable(t *testing.T) {
	labels := map[string]string{"app": "tolerant-app"}
	deploy := makeDeployment("tolerant-app", "default", 3, 3, labels)
	pdb := makePDBMaxUnavailable("pdb-tolerant", "default", "tolerant-app", 1)
	checker := safety.NewPDBChecker(fake.NewSimpleClientset(deploy, pdb))

	budget, err := checker.CalculateSafeDisruptionBudget(context.Background(), "default", "Deployment", "tolerant-app")
	if err != nil {
		t.Fatalf("CalculateSafeDisruptionBudget: %v", err)
	}
	// maxUnavailable(1) - currentUnavailable(3-3=0) = 1
	if budget != 1 {
		t.Errorf("expected budget=1, got %d", budget)
	}
}

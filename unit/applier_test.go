package unit_test

import (
	"context"
	"testing"

	"intelligent-cluster-optimizer/pkg/applier"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func makeApplierDeployment(name, ns, container, cpu, mem string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  container,
						Image: "nginx",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpu),
								corev1.ResourceMemory: resource.MustParse(mem),
							},
						},
					}},
				},
			},
		},
	}
}

// ─── ResourceRecommendation ───────────────────────────────────────────────────

func TestResourceRecommendation_HasChanges_True(t *testing.T) {
	r := &applier.ResourceRecommendation{
		CurrentCPU:        "100m",
		RecommendedCPU:    "200m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "128Mi",
	}
	if !r.HasChanges() {
		t.Error("expected HasChanges=true when CPU differs")
	}
}

func TestResourceRecommendation_HasChanges_False(t *testing.T) {
	r := &applier.ResourceRecommendation{
		CurrentCPU:        "100m",
		RecommendedCPU:    "100m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "128Mi",
	}
	if r.HasChanges() {
		t.Error("expected HasChanges=false when nothing differs")
	}
}

func TestResourceRecommendation_GetResourceRequirements(t *testing.T) {
	r := &applier.ResourceRecommendation{}
	req := r.GetResourceRequirements()
	if req.Requests == nil {
		t.Error("expected non-nil Requests")
	}
}

// ─── ParseResourceQuantity ────────────────────────────────────────────────────

func TestParseResourceQuantity_Valid(t *testing.T) {
	_, err := applier.ParseResourceQuantity("500m")
	if err != nil {
		t.Fatalf("expected valid parse: %v", err)
	}
}

func TestParseResourceQuantity_Invalid(t *testing.T) {
	_, err := applier.ParseResourceQuantity("not-a-quantity")
	if err == nil {
		t.Fatal("expected error for invalid quantity")
	}
}

// ─── DryRunApply ──────────────────────────────────────────────────────────────

func TestApplier_DryRun_NoChanges(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "dep",
		ContainerName:     "app",
		CurrentCPU:        "100m",
		RecommendedCPU:    "100m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "128Mi",
	}
	result, err := a.DryRunApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied {
		t.Error("expected Applied=false for dry run")
	}
	if !result.DryRun {
		t.Error("expected DryRun=true")
	}
	if len(result.Changes) != 0 {
		t.Errorf("expected no changes, got %v", result.Changes)
	}
}

func TestApplier_DryRun_CPUChange(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "dep",
		ContainerName:     "app",
		CurrentCPU:        "100m",
		RecommendedCPU:    "300m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "128Mi",
	}
	result, err := a.DryRunApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Changes) != 1 {
		t.Errorf("expected 1 change, got %d: %v", len(result.Changes), result.Changes)
	}
}

func TestApplier_DryRun_BothChanges(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		CurrentCPU:        "100m",
		RecommendedCPU:    "500m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "512Mi",
	}
	result, err := a.DryRunApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(result.Changes))
	}
}

func TestApplier_Apply_DryRunTrue(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		CurrentCPU: "100m", RecommendedCPU: "200m",
		CurrentMemory: "128Mi", RecommendedMemory: "128Mi",
	}
	result, err := a.Apply(context.Background(), rec, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied {
		t.Error("dry run should not have Applied=true")
	}
}

// ─── LiveApply ────────────────────────────────────────────────────────────────

func TestApplier_LiveApply_NoChanges(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "dep",
		ContainerName:     "app",
		CurrentCPU:        "100m",
		RecommendedCPU:    "100m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "128Mi",
	}
	result, err := a.LiveApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied {
		t.Error("expected Applied=false when no changes")
	}
}

func TestApplier_LiveApply_WithChanges(t *testing.T) {
	replicas := int32(1)
	dep := makeApplierDeployment("dep", "default", "app", "100m", "128Mi")
	dep.Spec.Replicas = &replicas
	dep.Generation = 1
	dep.Status = appsv1.DeploymentStatus{
		Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1, ObservedGeneration: 1,
	}
	client := fake.NewSimpleClientset(dep)
	a := applier.NewApplier(client, nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "dep",
		ContainerName:     "app",
		CurrentCPU:        "100m",
		RecommendedCPU:    "300m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "256Mi",
	}
	result, err := a.LiveApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("LiveApply failed: %v", err)
	}
	if !result.Applied {
		t.Error("expected Applied=true")
	}
	if len(result.Changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(result.Changes))
	}
}

func TestApplier_Apply_LiveFalse(t *testing.T) {
	replicas := int32(1)
	dep := makeApplierDeployment("dep", "default", "app", "100m", "128Mi")
	dep.Spec.Replicas = &replicas
	dep.Generation = 1
	dep.Status = appsv1.DeploymentStatus{
		Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1, ObservedGeneration: 1,
	}
	client := fake.NewSimpleClientset(dep)
	a := applier.NewApplier(client, nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "dep",
		ContainerName:     "app",
		CurrentCPU:        "100m",
		RecommendedCPU:    "200m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "128Mi",
	}
	result, err := a.Apply(context.Background(), rec, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Applied {
		t.Error("expected Applied=true for live apply")
	}
}

// ─── GetCurrentResources ──────────────────────────────────────────────────────

func TestApplier_GetCurrentResources_Deployment(t *testing.T) {
	dep := makeApplierDeployment("dep", "default", "app", "200m", "256Mi")
	a := applier.NewApplier(fake.NewSimpleClientset(dep), nil)
	rec, err := a.GetCurrentResources(context.Background(), "default", "Deployment", "dep", "app")
	if err != nil {
		t.Fatalf("GetCurrentResources failed: %v", err)
	}
	if rec.CurrentCPU != "200m" {
		t.Errorf("expected CPU=200m, got %s", rec.CurrentCPU)
	}
	if rec.CurrentMemory != "256Mi" {
		t.Errorf("expected Memory=256Mi, got %s", rec.CurrentMemory)
	}
}

func TestApplier_GetCurrentResources_StatefulSet(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "sts"}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "nginx",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
					}},
				},
			},
		},
	}
	a := applier.NewApplier(fake.NewSimpleClientset(sts), nil)
	rec, err := a.GetCurrentResources(context.Background(), "default", "StatefulSet", "sts", "app")
	if err != nil {
		t.Fatalf("GetCurrentResources StatefulSet failed: %v", err)
	}
	if rec.CurrentCPU != "100m" {
		t.Errorf("expected 100m, got %s", rec.CurrentCPU)
	}
}

func TestApplier_GetCurrentResources_DaemonSet(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ds"}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "nginx",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("32Mi"),
							},
						},
					}},
				},
			},
		},
	}
	a := applier.NewApplier(fake.NewSimpleClientset(ds), nil)
	rec, err := a.GetCurrentResources(context.Background(), "default", "DaemonSet", "ds", "app")
	if err != nil {
		t.Fatalf("GetCurrentResources DaemonSet failed: %v", err)
	}
	if rec.CurrentCPU != "50m" {
		t.Errorf("expected 50m, got %s", rec.CurrentCPU)
	}
}

func TestApplier_GetCurrentResources_UnsupportedKind(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	_, err := a.GetCurrentResources(context.Background(), "default", "Job", "job1", "app")
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestApplier_GetCurrentResources_NotFound(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	_, err := a.GetCurrentResources(context.Background(), "default", "Deployment", "nonexistent", "app")
	if err == nil {
		t.Fatal("expected error for nonexistent deployment")
	}
}

func TestApplier_GetCurrentResources_ContainerNotFound(t *testing.T) {
	dep := makeApplierDeployment("dep", "default", "app", "100m", "128Mi")
	a := applier.NewApplier(fake.NewSimpleClientset(dep), nil)
	// container "sidecar" doesn't exist → returns rec with empty CPU/memory (no error)
	rec, err := a.GetCurrentResources(context.Background(), "default", "Deployment", "dep", "sidecar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.CurrentCPU != "" {
		t.Errorf("expected empty CPU for missing container, got %s", rec.CurrentCPU)
	}
}

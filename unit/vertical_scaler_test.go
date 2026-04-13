package unit_test

import (
	"context"
	"testing"

	"intelligent-cluster-optimizer/pkg/scaler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func makeScalerDeployment(name, ns, container, cpu, mem string) *appsv1.Deployment {
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

func makeScalerStatefulSet(name, ns, container, cpu, mem string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
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

func makeScalerDaemonSet(name, ns, container, cpu, mem string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DaemonSetSpec{
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

// ─── DetectInPlaceSupport ─────────────────────────────────────────────────────

func TestVerticalScaler_DetectInPlaceSupport(t *testing.T) {
	client := fake.NewSimpleClientset()
	vs := scaler.NewVerticalScaler(client, nil)
	supported, err := vs.DetectInPlaceSupport(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// fake client returns false for in-place (not 1.27+)
	_ = supported
}

// ─── ApplyInPlaceUpdate ───────────────────────────────────────────────────────

func TestVerticalScaler_ApplyInPlaceUpdate_Deployment(t *testing.T) {
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace:     "default",
		WorkloadKind:  "Deployment",
		WorkloadName:  "dep",
		ContainerName: "app",
		NewCPU:        "200m",
		NewMemory:     "256Mi",
		Strategy:      scaler.StrategyInPlace,
	}
	if err := vs.ApplyInPlaceUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyInPlaceUpdate Deployment failed: %v", err)
	}
}

func TestVerticalScaler_ApplyInPlaceUpdate_StatefulSet(t *testing.T) {
	replicas := int32(1)
	sts := makeScalerStatefulSet("sts", "default", "app", "100m", "128Mi", replicas)
	client := fake.NewSimpleClientset(sts)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "StatefulSet", WorkloadName: "sts",
		ContainerName: "app", NewCPU: "300m", Strategy: scaler.StrategyInPlace,
	}
	if err := vs.ApplyInPlaceUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyInPlaceUpdate StatefulSet failed: %v", err)
	}
}

func TestVerticalScaler_ApplyInPlaceUpdate_DaemonSet(t *testing.T) {
	ds := makeScalerDaemonSet("ds", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(ds)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "DaemonSet", WorkloadName: "ds",
		ContainerName: "app", NewCPU: "150m", Strategy: scaler.StrategyInPlace,
	}
	if err := vs.ApplyInPlaceUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyInPlaceUpdate DaemonSet failed: %v", err)
	}
}

func TestVerticalScaler_ApplyInPlaceUpdate_UnsupportedKind(t *testing.T) {
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(), nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Job", WorkloadName: "job",
		ContainerName: "app",
	}
	err := vs.ApplyInPlaceUpdate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestVerticalScaler_ApplyInPlaceUpdate_ContainerNotFound(t *testing.T) {
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(dep), nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "dep",
		ContainerName: "sidecar", NewCPU: "200m",
	}
	err := vs.ApplyInPlaceUpdate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for missing container")
	}
}

func TestVerticalScaler_ApplyInPlaceUpdate_InvalidCPU(t *testing.T) {
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(dep), nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "dep",
		ContainerName: "app", NewCPU: "not-a-cpu",
	}
	err := vs.ApplyInPlaceUpdate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid CPU")
	}
}

func TestVerticalScaler_ApplyInPlaceUpdate_InvalidMemory(t *testing.T) {
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(dep), nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "dep",
		ContainerName: "app", NewCPU: "200m", NewMemory: "not-mem",
	}
	err := vs.ApplyInPlaceUpdate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid memory")
	}
}

// ─── ApplyRollingUpdate ───────────────────────────────────────────────────────

func TestVerticalScaler_ApplyRollingUpdate_UnsupportedKind(t *testing.T) {
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(), nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "CronJob", WorkloadName: "cj",
		ContainerName: "app",
	}
	err := vs.ApplyRollingUpdate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unsupported kind in rolling update")
	}
}

func TestVerticalScaler_ApplyRollingUpdate_Deployment(t *testing.T) {
	replicas := int32(1)
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	dep.Spec.Replicas = &replicas
	dep.Status.Replicas = 1
	dep.Status.UpdatedReplicas = 1
	dep.Status.AvailableReplicas = 1
	dep.Generation = 1
	dep.Status.ObservedGeneration = 1

	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "dep",
		ContainerName: "app", NewCPU: "300m", NewMemory: "256Mi",
		Strategy: scaler.StrategyRolling,
	}
	// Rolling update will poll for readiness. With fake client the deployment
	// stays in the state we set (ready), so it should complete quickly.
	if err := vs.ApplyRollingUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyRollingUpdate Deployment failed: %v", err)
	}
}

func TestVerticalScaler_ApplyRollingUpdate_DaemonSet(t *testing.T) {
	ds := makeScalerDaemonSet("ds", "default", "app", "100m", "128Mi")
	ds.Status.DesiredNumberScheduled = 1
	ds.Status.UpdatedNumberScheduled = 1
	ds.Status.NumberAvailable = 1
	ds.Generation = 1
	ds.Status.ObservedGeneration = 1

	client := fake.NewSimpleClientset(ds)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "DaemonSet", WorkloadName: "ds",
		ContainerName: "app", NewCPU: "200m", Strategy: scaler.StrategyRolling,
	}
	if err := vs.ApplyRollingUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyRollingUpdate DaemonSet failed: %v", err)
	}
}

// ─── Scale (strategy routing) ─────────────────────────────────────────────────

func TestVerticalScaler_Scale_InPlaceStrategy_FallsBackToRolling(t *testing.T) {
	// fake client's discovery returns a version that doesn't support in-place
	// so it falls back to rolling
	replicas := int32(1)
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	dep.Spec.Replicas = &replicas
	dep.Status = appsv1.DeploymentStatus{
		Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1, ObservedGeneration: 1,
	}
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "dep",
		ContainerName: "app", NewCPU: "200m", Strategy: scaler.StrategyInPlace,
	}
	if err := vs.Scale(context.Background(), req); err != nil {
		t.Fatalf("Scale with InPlace strategy failed: %v", err)
	}
}

func TestVerticalScaler_Scale_RollingStrategy(t *testing.T) {
	replicas := int32(1)
	dep := makeScalerDeployment("dep", "default", "app", "100m", "128Mi")
	dep.Spec.Replicas = &replicas
	dep.Status = appsv1.DeploymentStatus{
		Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1, ObservedGeneration: 1,
	}
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "dep",
		ContainerName: "app", NewCPU: "500m", Strategy: scaler.StrategyRolling,
	}
	if err := vs.Scale(context.Background(), req); err != nil {
		t.Fatalf("Scale rolling failed: %v", err)
	}
}

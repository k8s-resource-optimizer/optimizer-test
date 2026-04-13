package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"intelligent-cluster-optimizer/pkg/applier"
	"intelligent-cluster-optimizer/pkg/rollback"
	"intelligent-cluster-optimizer/pkg/scaler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func intDeployment(name, ns, container, cpu, mem string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  container,
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse(mem),
						},
					},
				}}},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           replicas,
			UpdatedReplicas:    replicas,
			AvailableReplicas:  replicas,
			ObservedGeneration: 1,
		},
	}
}

func intStatefulSet(name, ns, container, cpu, mem string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  container,
					Image: "redis",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse(mem),
						},
					},
				}}},
			},
		},
	}
}

func intDaemonSet(name, ns, container, cpu, mem string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  container,
					Image: "fluentd",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse(mem),
						},
					},
				}}},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 3,
			NumberAvailable:        3,
			ObservedGeneration:     1,
		},
	}
}

// ─── Applier integration ──────────────────────────────────────────────────────

func TestIntApplier_DryRun_NoChanges(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "app",
		ContainerName:     "app",
		CurrentCPU:        "200m",
		RecommendedCPU:    "200m",
		CurrentMemory:     "256Mi",
		RecommendedMemory: "256Mi",
	}
	r, err := a.DryRunApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("DryRunApply failed: %v", err)
	}
	if r.Applied || !r.DryRun || len(r.Changes) != 0 {
		t.Errorf("unexpected result: applied=%v dryRun=%v changes=%v", r.Applied, r.DryRun, r.Changes)
	}
}

func TestIntApplier_DryRun_CPUChange(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "prod",
		WorkloadKind:      "Deployment",
		WorkloadName:      "web",
		ContainerName:     "web",
		CurrentCPU:        "100m",
		RecommendedCPU:    "500m",
		CurrentMemory:     "256Mi",
		RecommendedMemory: "256Mi",
	}
	r, err := a.DryRunApply(context.Background(), rec)
	if err != nil {
		t.Fatalf("DryRunApply failed: %v", err)
	}
	if len(r.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(r.Changes), r.Changes)
	}
}

func TestIntApplier_DryRun_MemoryChange(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		CurrentCPU: "100m", RecommendedCPU: "100m",
		CurrentMemory: "128Mi", RecommendedMemory: "512Mi",
	}
	r, err := a.DryRunApply(context.Background(), rec)
	if err != nil || len(r.Changes) != 1 {
		t.Fatalf("expected 1 change: err=%v changes=%d", err, len(r.Changes))
	}
}

func TestIntApplier_DryRun_BothChanges(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		CurrentCPU: "100m", RecommendedCPU: "300m",
		CurrentMemory: "128Mi", RecommendedMemory: "512Mi",
	}
	r, _ := a.DryRunApply(context.Background(), rec)
	if len(r.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(r.Changes))
	}
}

func TestIntApplier_Apply_DryRunTrue(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	rec := &applier.ResourceRecommendation{
		CurrentCPU: "100m", RecommendedCPU: "200m",
		CurrentMemory: "128Mi", RecommendedMemory: "128Mi",
	}
	r, err := a.Apply(context.Background(), rec, true)
	if err != nil || r.Applied {
		t.Fatalf("expected no-apply in dry-run: err=%v applied=%v", err, r.Applied)
	}
}

func TestIntApplier_LiveApply_Deployment(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 2)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	a := applier.NewApplier(client, nil)
	rec := &applier.ResourceRecommendation{
		Namespace:         "default",
		WorkloadKind:      "Deployment",
		WorkloadName:      "web",
		ContainerName:     "web",
		CurrentCPU:        "100m",
		RecommendedCPU:    "300m",
		CurrentMemory:     "128Mi",
		RecommendedMemory: "256Mi",
	}
	r, err := a.Apply(context.Background(), rec, false)
	if err != nil {
		t.Fatalf("LiveApply failed: %v", err)
	}
	if !r.Applied {
		t.Error("expected Applied=true")
	}
	if len(r.Changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(r.Changes))
	}
}

func TestIntApplier_GetCurrentResources_Deployment(t *testing.T) {
	dep := intDeployment("api", "default", "api", "250m", "512Mi", 1)
	a := applier.NewApplier(fake.NewSimpleClientset(dep), nil)
	rec, err := a.GetCurrentResources(context.Background(), "default", "Deployment", "api", "api")
	if err != nil {
		t.Fatalf("GetCurrentResources failed: %v", err)
	}
	if rec.CurrentCPU != "250m" {
		t.Errorf("expected 250m, got %s", rec.CurrentCPU)
	}
	if rec.CurrentMemory != "512Mi" {
		t.Errorf("expected 512Mi, got %s", rec.CurrentMemory)
	}
}

func TestIntApplier_GetCurrentResources_StatefulSet(t *testing.T) {
	sts := intStatefulSet("db", "default", "db", "500m", "1Gi", 3)
	a := applier.NewApplier(fake.NewSimpleClientset(sts), nil)
	rec, err := a.GetCurrentResources(context.Background(), "default", "StatefulSet", "db", "db")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if rec.CurrentCPU != "500m" {
		t.Errorf("expected 500m, got %s", rec.CurrentCPU)
	}
}

func TestIntApplier_GetCurrentResources_DaemonSet(t *testing.T) {
	ds := intDaemonSet("log-collector", "default", "log-collector", "50m", "64Mi")
	a := applier.NewApplier(fake.NewSimpleClientset(ds), nil)
	rec, err := a.GetCurrentResources(context.Background(), "default", "DaemonSet", "log-collector", "log-collector")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if rec.CurrentCPU != "50m" {
		t.Errorf("expected 50m, got %s", rec.CurrentCPU)
	}
}

func TestIntApplier_GetCurrentResources_NotFound(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	_, err := a.GetCurrentResources(context.Background(), "default", "Deployment", "nonexistent", "app")
	if err == nil {
		t.Fatal("expected error for nonexistent workload")
	}
}

func TestIntApplier_GetCurrentResources_UnsupportedKind(t *testing.T) {
	a := applier.NewApplier(fake.NewSimpleClientset(), nil)
	_, err := a.GetCurrentResources(context.Background(), "default", "CronJob", "j1", "app")
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestIntApplier_ParseResourceQuantity(t *testing.T) {
	tests := []struct {
		val     string
		wantErr bool
	}{
		{"100m", false},
		{"500m", false},
		{"2", false},
		{"128Mi", false},
		{"1Gi", false},
		{"not-valid", true},
	}
	for _, tc := range tests {
		_, err := applier.ParseResourceQuantity(tc.val)
		if tc.wantErr && err == nil {
			t.Errorf("expected error for %q", tc.val)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("unexpected error for %q: %v", tc.val, err)
		}
	}
}

// ─── RollbackManager integration ──────────────────────────────────────────────

func TestIntRollback_SaveAndRollback_Deployment(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 2)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)

	// Save initial config
	if err := rm.SavePreviousConfig(context.Background(), "default", "Deployment", "web", "web"); err != nil {
		t.Fatalf("SavePreviousConfig failed: %v", err)
	}

	// Simulate change
	current, _ := client.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	current.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("500m")
	client.AppsV1().Deployments("default").Update(context.Background(), current, metav1.UpdateOptions{})

	// Save again (now we have 2 entries)
	if err := rm.SavePreviousConfig(context.Background(), "default", "Deployment", "web", "web"); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}
	if rm.GetHistoryCount() < 2 {
		t.Fatalf("expected at least 2 history entries, got %d", rm.GetHistoryCount())
	}

	// Rollback
	if err := rm.RollbackWorkload(context.Background(), "default", "Deployment", "web", "web"); err != nil {
		t.Fatalf("RollbackWorkload failed: %v", err)
	}
}

func TestIntRollback_SaveAndRollback_StatefulSet(t *testing.T) {
	sts := intStatefulSet("db", "default", "db", "200m", "256Mi", 3)
	client := fake.NewSimpleClientset(sts)
	rm := rollback.NewRollbackManager(client)

	rm.SavePreviousConfig(context.Background(), "default", "StatefulSet", "db", "db")

	current, _ := client.AppsV1().StatefulSets("default").Get(context.Background(), "db", metav1.GetOptions{})
	current.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("800m")
	client.AppsV1().StatefulSets("default").Update(context.Background(), current, metav1.UpdateOptions{})
	rm.SavePreviousConfig(context.Background(), "default", "StatefulSet", "db", "db")

	if err := rm.RollbackWorkload(context.Background(), "default", "StatefulSet", "db", "db"); err != nil {
		t.Fatalf("RollbackWorkload StatefulSet failed: %v", err)
	}
}

func TestIntRollback_SaveAndRollback_DaemonSet(t *testing.T) {
	ds := intDaemonSet("agent", "default", "agent", "50m", "64Mi")
	client := fake.NewSimpleClientset(ds)
	rm := rollback.NewRollbackManager(client)

	rm.SavePreviousConfig(context.Background(), "default", "DaemonSet", "agent", "agent")
	current, _ := client.AppsV1().DaemonSets("default").Get(context.Background(), "agent", metav1.GetOptions{})
	current.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("200m")
	client.AppsV1().DaemonSets("default").Update(context.Background(), current, metav1.UpdateOptions{})
	rm.SavePreviousConfig(context.Background(), "default", "DaemonSet", "agent", "agent")

	if err := rm.RollbackWorkload(context.Background(), "default", "DaemonSet", "agent", "agent"); err != nil {
		t.Fatalf("RollbackWorkload DaemonSet failed: %v", err)
	}
}

func TestIntRollback_NoPrevious(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	err := rm.RollbackWorkload(context.Background(), "ns", "Deployment", "dep", "app")
	if err == nil {
		t.Fatal("expected error for no previous config")
	}
}

func TestIntRollback_UnsupportedKind(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	err := rm.SavePreviousConfig(context.Background(), "ns", "CronJob", "j", "app")
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestIntRollback_GetAllHistory_MultipleWorkloads(t *testing.T) {
	dep1 := intDeployment("dep1", "default", "app", "100m", "128Mi", 1)
	dep2 := intDeployment("dep2", "default", "app", "200m", "256Mi", 2)
	client := fake.NewSimpleClientset(dep1, dep2)
	rm := rollback.NewRollbackManager(client)
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep1", "app")
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep2", "app")

	all := rm.GetAllHistory()
	if len(all) != 2 {
		t.Fatalf("expected 2 workloads in history, got %d", len(all))
	}
}

func TestIntRollback_GetWorkloadHistory(t *testing.T) {
	dep := intDeployment("dep", "default", "app", "100m", "128Mi", 1)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")
	hist := rm.GetWorkloadHistory("default", "Deployment", "dep", "app")
	if len(hist) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(hist))
	}
}

func TestIntRollback_MaxHistory(t *testing.T) {
	dep := intDeployment("dep", "default", "app", "100m", "128Mi", 1)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)
	for i := 0; i < rollback.MaxHistoryPerWorkload+3; i++ {
		rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")
	}
	hist := rm.GetWorkloadHistory("default", "Deployment", "dep", "app")
	if len(hist) > rollback.MaxHistoryPerWorkload {
		t.Fatalf("expected max %d entries, got %d", rollback.MaxHistoryPerWorkload, len(hist))
	}
}

func TestIntRollback_SaveLoadFile(t *testing.T) {
	dep := intDeployment("dep", "default", "app", "100m", "128Mi", 1)
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")

	path := filepath.Join(t.TempDir(), "history.json")
	if err := rm.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	rm2 := rollback.NewRollbackManager(fake.NewSimpleClientset())
	if err := rm2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if rm2.GetHistoryCount() != rm.GetHistoryCount() {
		t.Errorf("count mismatch: %d vs %d", rm2.GetHistoryCount(), rm.GetHistoryCount())
	}
}

func TestIntRollback_LoadFile_NotExist(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	if err := rm.LoadFromFile("/no/such/file/xyz.json"); err != nil {
		t.Fatalf("non-existent file should not error: %v", err)
	}
}

func TestIntRollback_LoadFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte("{bad}"), 0600)
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	if err := rm.LoadFromFile(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ─── VerticalScaler integration ───────────────────────────────────────────────

func TestIntScaler_ApplyInPlace_Deployment(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 2)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "web",
		ContainerName: "web", NewCPU: "300m", NewMemory: "256Mi",
		Strategy: scaler.StrategyInPlace,
	}
	if err := vs.ApplyInPlaceUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyInPlaceUpdate failed: %v", err)
	}
}

func TestIntScaler_ApplyInPlace_StatefulSet(t *testing.T) {
	sts := intStatefulSet("db", "default", "db", "200m", "256Mi", 1)
	client := fake.NewSimpleClientset(sts)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "StatefulSet", WorkloadName: "db",
		ContainerName: "db", NewCPU: "500m",
	}
	if err := vs.ApplyInPlaceUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyInPlaceUpdate StatefulSet failed: %v", err)
	}
}

func TestIntScaler_ApplyInPlace_DaemonSet(t *testing.T) {
	ds := intDaemonSet("log", "default", "log", "50m", "64Mi")
	client := fake.NewSimpleClientset(ds)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "DaemonSet", WorkloadName: "log",
		ContainerName: "log", NewCPU: "100m", NewMemory: "128Mi",
	}
	if err := vs.ApplyInPlaceUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyInPlaceUpdate DaemonSet failed: %v", err)
	}
}

func TestIntScaler_ApplyInPlace_UnsupportedKind(t *testing.T) {
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(), nil)
	err := vs.ApplyInPlaceUpdate(context.Background(), &scaler.ScaleRequest{WorkloadKind: "Job"})
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestIntScaler_ApplyInPlace_ContainerNotFound(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 1)
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(dep), nil)
	err := vs.ApplyInPlaceUpdate(context.Background(), &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "web",
		ContainerName: "nonexistent", NewCPU: "200m",
	})
	if err == nil {
		t.Fatal("expected error for missing container")
	}
}

func TestIntScaler_ApplyInPlace_InvalidCPU(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 1)
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(dep), nil)
	err := vs.ApplyInPlaceUpdate(context.Background(), &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "web",
		ContainerName: "web", NewCPU: "not-cpu",
	})
	if err == nil {
		t.Fatal("expected error for invalid CPU")
	}
}

func TestIntScaler_ApplyInPlace_InvalidMemory(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 1)
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(dep), nil)
	err := vs.ApplyInPlaceUpdate(context.Background(), &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "web",
		ContainerName: "web", NewCPU: "200m", NewMemory: "bad-mem",
	})
	if err == nil {
		t.Fatal("expected error for invalid memory")
	}
}

func TestIntScaler_RollingUpdate_Deployment(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 2)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "web",
		ContainerName: "web", NewCPU: "400m", NewMemory: "512Mi",
		Strategy: scaler.StrategyRolling,
	}
	if err := vs.ApplyRollingUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyRollingUpdate Deployment failed: %v", err)
	}
}

func TestIntScaler_RollingUpdate_DaemonSet(t *testing.T) {
	ds := intDaemonSet("log", "default", "log", "50m", "64Mi")
	ds.Generation = 1
	client := fake.NewSimpleClientset(ds)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "DaemonSet", WorkloadName: "log",
		ContainerName: "log", NewCPU: "100m",
		Strategy: scaler.StrategyRolling,
	}
	if err := vs.ApplyRollingUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplyRollingUpdate DaemonSet failed: %v", err)
	}
}

func TestIntScaler_RollingUpdate_UnsupportedKind(t *testing.T) {
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(), nil)
	err := vs.ApplyRollingUpdate(context.Background(), &scaler.ScaleRequest{WorkloadKind: "Job"})
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestIntScaler_Scale_FallsBackToRolling(t *testing.T) {
	dep := intDeployment("web", "default", "web", "100m", "128Mi", 1)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)
	vs := scaler.NewVerticalScaler(client, nil)
	req := &scaler.ScaleRequest{
		Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "web",
		ContainerName: "web", NewCPU: "200m",
		Strategy: scaler.StrategyInPlace, // in-place not supported → rolling
	}
	if err := vs.Scale(context.Background(), req); err != nil {
		t.Fatalf("Scale (InPlace fallback) failed: %v", err)
	}
}

func TestIntScaler_DetectInPlaceSupport(t *testing.T) {
	vs := scaler.NewVerticalScaler(fake.NewSimpleClientset(), nil)
	_, err := vs.DetectInPlaceSupport(context.Background())
	if err != nil {
		t.Fatalf("DetectInPlaceSupport failed: %v", err)
	}
}

func TestIntScaler_FullPipeline_OptimizeDeployment(t *testing.T) {
	// Full pipeline: get current → dry run → live apply → save rollback → verify
	dep := intDeployment("api", "prod", "api", "150m", "256Mi", 3)
	dep.Generation = 1
	client := fake.NewSimpleClientset(dep)

	a := applier.NewApplier(client, nil)
	rm := rollback.NewRollbackManager(client)

	// 1. Get current resources
	current, err := a.GetCurrentResources(context.Background(), "prod", "Deployment", "api", "api")
	if err != nil {
		t.Fatalf("GetCurrentResources: %v", err)
	}
	if current.CurrentCPU == "" {
		t.Fatal("CurrentCPU should not be empty")
	}

	// 2. Simulate recommendation
	rec := &applier.ResourceRecommendation{
		Namespace:         "prod",
		WorkloadKind:      "Deployment",
		WorkloadName:      "api",
		ContainerName:     "api",
		CurrentCPU:        current.CurrentCPU,
		RecommendedCPU:    "300m",
		CurrentMemory:     current.CurrentMemory,
		RecommendedMemory: "512Mi",
	}

	// 3. Dry run first
	dryResult, err := a.Apply(context.Background(), rec, true)
	if err != nil || dryResult.Applied {
		t.Fatalf("dry run should not apply: err=%v applied=%v", err, dryResult.Applied)
	}

	// 4. Save rollback config before applying
	if err := rm.SavePreviousConfig(context.Background(), "prod", "Deployment", "api", "api"); err != nil {
		t.Fatalf("SavePreviousConfig: %v", err)
	}

	// 5. Live apply
	liveResult, err := a.Apply(context.Background(), rec, false)
	if err != nil {
		t.Fatalf("LiveApply: %v", err)
	}
	if !liveResult.Applied {
		t.Error("expected Applied=true")
	}

	// 6. Save post-apply config for future rollback
	rm.SavePreviousConfig(context.Background(), "prod", "Deployment", "api", "api")
	if rm.GetHistoryCount() < 2 {
		t.Errorf("expected at least 2 history entries, got %d", rm.GetHistoryCount())
	}
}

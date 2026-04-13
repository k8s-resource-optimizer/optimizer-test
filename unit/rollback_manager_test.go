package unit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"intelligent-cluster-optimizer/pkg/rollback"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// makeDeploymentWithResources creates a fake Deployment with known CPU/memory.
func makeDeploymentWithResources(name, ns, container, cpu, mem string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
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

// ─── SavePreviousConfig ───────────────────────────────────────────────────────

func TestRollback_SavePreviousConfig_Deployment(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)

	err := rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")
	if err != nil {
		t.Fatalf("SavePreviousConfig failed: %v", err)
	}
	if rm.GetHistoryCount() != 1 {
		t.Fatalf("expected 1 history entry, got %d", rm.GetHistoryCount())
	}
}

func TestRollback_SavePreviousConfig_StatefulSet(t *testing.T) {
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
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					}},
				},
			},
		},
	}
	client := fake.NewSimpleClientset(sts)
	rm := rollback.NewRollbackManager(client)
	err := rm.SavePreviousConfig(context.Background(), "default", "StatefulSet", "sts", "app")
	if err != nil {
		t.Fatalf("SavePreviousConfig StatefulSet failed: %v", err)
	}
}

func TestRollback_SavePreviousConfig_DaemonSet(t *testing.T) {
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
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
					}},
				},
			},
		},
	}
	client := fake.NewSimpleClientset(ds)
	rm := rollback.NewRollbackManager(client)
	err := rm.SavePreviousConfig(context.Background(), "default", "DaemonSet", "ds", "app")
	if err != nil {
		t.Fatalf("SavePreviousConfig DaemonSet failed: %v", err)
	}
}

func TestRollback_SavePreviousConfig_UnsupportedKind(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	err := rm.SavePreviousConfig(context.Background(), "default", "Job", "job1", "app")
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestRollback_SavePreviousConfig_ContainerNotFound(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset(dep))
	err := rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "nonexistent")
	if err == nil {
		t.Fatal("expected error when container not found")
	}
}

func TestRollback_SavePreviousConfig_MaxHistory(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)

	// Save MaxHistoryPerWorkload+2 configs
	for i := 0; i < rollback.MaxHistoryPerWorkload+2; i++ {
		rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")
	}
	hist := rm.GetWorkloadHistory("default", "Deployment", "dep", "app")
	if len(hist) > rollback.MaxHistoryPerWorkload {
		t.Fatalf("history should be capped at %d, got %d", rollback.MaxHistoryPerWorkload, len(hist))
	}
}

// ─── RollbackWorkload ─────────────────────────────────────────────────────────

func TestRollback_RollbackWorkload_Success(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)

	// Save twice so rollback has a previous entry
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")

	// Simulate update (change resources)
	dep2, _ := client.AppsV1().Deployments("default").Get(context.Background(), "dep", metav1.GetOptions{})
	dep2.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("500m")
	client.AppsV1().Deployments("default").Update(context.Background(), dep2, metav1.UpdateOptions{})

	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")

	err := rm.RollbackWorkload(context.Background(), "default", "Deployment", "dep", "app")
	if err != nil {
		t.Fatalf("RollbackWorkload failed: %v", err)
	}
}

func TestRollback_RollbackWorkload_NoPreviousConfig(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	err := rm.RollbackWorkload(context.Background(), "default", "Deployment", "dep", "app")
	if err == nil {
		t.Fatal("expected error when no previous config")
	}
}

func TestRollback_RollbackWorkload_OnlyOneEntry(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")

	// Only 1 entry → not enough to rollback (need ≥2)
	err := rm.RollbackWorkload(context.Background(), "default", "Deployment", "dep", "app")
	if err == nil {
		t.Fatal("expected error: only 1 history entry, need 2 for rollback")
	}
}

// ─── GetAllHistory / GetWorkloadHistory / GetHistoryCount ─────────────────────

func TestRollback_GetAllHistory(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset(dep))
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")

	all := rm.GetAllHistory()
	if len(all) == 0 {
		t.Fatal("expected at least one history entry")
	}
}

func TestRollback_GetWorkloadHistory_Empty(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	hist := rm.GetWorkloadHistory("ns", "Deployment", "dep", "app")
	if len(hist) != 0 {
		t.Fatalf("expected empty history, got %d", len(hist))
	}
}

func TestRollback_GetHistoryCount_Empty(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	if rm.GetHistoryCount() != 0 {
		t.Error("expected 0 history count")
	}
}

// ─── SaveToFile / LoadFromFile ────────────────────────────────────────────────

func TestRollback_SaveAndLoadFile(t *testing.T) {
	dep := makeDeploymentWithResources("dep", "default", "app", "100m", "128Mi")
	client := fake.NewSimpleClientset(dep)
	rm := rollback.NewRollbackManager(client)
	rm.SavePreviousConfig(context.Background(), "default", "Deployment", "dep", "app")

	tmpFile := filepath.Join(t.TempDir(), "rollback.json")
	if err := rm.SaveToFile(tmpFile); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	rm2 := rollback.NewRollbackManager(fake.NewSimpleClientset())
	if err := rm2.LoadFromFile(tmpFile); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	if rm2.GetHistoryCount() != rm.GetHistoryCount() {
		t.Errorf("loaded history count %d != saved %d", rm2.GetHistoryCount(), rm.GetHistoryCount())
	}
}

func TestRollback_LoadFromFile_NotExist(t *testing.T) {
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	// Non-existent file should return nil (not an error)
	err := rm.LoadFromFile("/tmp/no-such-file-rollback-xyz.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
}

func TestRollback_LoadFromFile_InvalidJSON(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(tmpFile, []byte("not-json"), 0600)
	rm := rollback.NewRollbackManager(fake.NewSimpleClientset())
	err := rm.LoadFromFile(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ─── WorkloadConfig.Key ───────────────────────────────────────────────────────

func TestWorkloadConfig_Key(t *testing.T) {
	wc := &rollback.WorkloadConfig{
		Namespace:     "ns",
		Kind:          "Deployment",
		Name:          "dep",
		ContainerName: "app",
	}
	expected := "ns/Deployment/dep/app"
	if wc.Key() != expected {
		t.Errorf("expected %q, got %q", expected, wc.Key())
	}
}

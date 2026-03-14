package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// restConfig builds a *rest.Config from KUBECONFIG or ~/.kube/config.
// Tests are skipped if no cluster is reachable.
func restConfig(t *testing.T) *clientcmd.ClientConfig {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{},
	)
	return &cc
}

// dynamicClient returns a dynamic Kubernetes client, skipping if unavailable.
func dynamicClient(t *testing.T) dynamic.Interface {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Skipf("skipping E2E: cannot build kubeconfig: %v", err)
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Skipf("skipping E2E: cannot create dynamic client: %v", err)
	}
	return dc
}

// optimizerConfigClient returns a typed OptimizerConfigClient.
func optimizerConfigClient(t *testing.T, namespace string) *v1alpha1.OptimizerConfigClient {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Skipf("skipping E2E: cannot build kubeconfig: %v", err)
	}
	client, err := v1alpha1.NewOptimizerConfigClient(cfg, namespace)
	if err != nil {
		t.Skipf("skipping E2E: cannot create OptimizerConfigClient: %v", err)
	}
	return client
}

// TestE2E_OptimizerConfigLifecycle tests the full CRD lifecycle:
// Create → List → Get → Watch → Update → Delete
// This exercises pkg/apis/optimizer/v1alpha1 client code.
func TestE2E_OptimizerConfigLifecycle(t *testing.T) {
	const ns = controllerNamespace
	ctx := context.Background()

	dc := dynamicClient(t)
	oc := optimizerConfigClient(t, ns)
	gvr := v1alpha1.GroupVersionResource()

	// ── 1. Create ──────────────────────────────────────────────────────────────
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "optimizer.cluster.io/v1alpha1",
			"kind":       "OptimizerConfig",
			"metadata": map[string]interface{}{
				"name":      "e2e-lifecycle-test",
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"enabled":          true,
				"targetNamespaces": []interface{}{testNamespace},
				"strategy":         "balanced",
				"dryRun":           true,
			},
		},
	}

	created, err := dc.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create OptimizerConfig: %v", err)
	}
	t.Cleanup(func() {
		_ = dc.Resource(gvr).Namespace(ns).Delete(context.Background(), "e2e-lifecycle-test", metav1.DeleteOptions{})
	})
	t.Logf("Created OptimizerConfig: %s", created.GetName())

	// ── 2. List ────────────────────────────────────────────────────────────────
	list, err := oc.List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List OptimizerConfigs: %v", err)
	}
	found := false
	for _, item := range list.Items {
		if item.Name == "e2e-lifecycle-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Created OptimizerConfig not found in List")
	}

	// ── 3. Get ─────────────────────────────────────────────────────────────────
	got, err := oc.Get(ctx, "e2e-lifecycle-test", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get OptimizerConfig: %v", err)
	}
	if got.Spec.Strategy != v1alpha1.StrategyBalanced {
		t.Errorf("expected strategy=balanced, got %s", got.Spec.Strategy)
	}
	if !got.Spec.DryRun {
		t.Error("expected dryRun=true")
	}

	// ── 4. Watch ───────────────────────────────────────────────────────────────
	watcher, err := oc.Watch(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Watch OptimizerConfigs: %v", err)
	}
	watcher.Stop()

	// ── 5. Update (patch strategy field) ──────────────────────────────────────
	latest, err := dc.Resource(gvr).Namespace(ns).Get(ctx, "e2e-lifecycle-test", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("re-fetch for update: %v", err)
	}
	if err := unstructured.SetNestedField(latest.Object, "conservative", "spec", "strategy"); err != nil {
		t.Fatalf("set strategy field: %v", err)
	}
	updated, err := dc.Resource(gvr).Namespace(ns).Update(ctx, latest, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Update OptimizerConfig: %v", err)
	}
	strategy, _, _ := unstructured.NestedString(updated.Object, "spec", "strategy")
	if strategy != "conservative" {
		t.Errorf("expected strategy=conservative after update, got %s", strategy)
	}
	t.Logf("Updated OptimizerConfig strategy to: %s", strategy)

	// ── 6. Verify controller processed it (still Running) ─────────────────────
	kc := kubeClient(t)
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 15*time.Second, true,
		func(ctx context.Context) (bool, error) {
			dep, err := kc.AppsV1().Deployments(controllerNamespace).
				Get(ctx, controllerDeployment, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			return dep.Status.AvailableReplicas >= 1, nil
		},
	)
	if err != nil {
		t.Error("controller became unavailable after OptimizerConfig create/update")
	}

	// ── 7. Delete ──────────────────────────────────────────────────────────────
	if err := dc.Resource(gvr).Namespace(ns).Delete(ctx, "e2e-lifecycle-test", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete OptimizerConfig: %v", err)
	}
	t.Log("Deleted OptimizerConfig successfully")

	// ── 8. Verify deletion ─────────────────────────────────────────────────────
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 10*time.Second, true,
		func(ctx context.Context) (bool, error) {
			_, err := oc.Get(ctx, "e2e-lifecycle-test", metav1.GetOptions{})
			return err != nil, nil // gone when err != nil
		},
	)
	if err != nil {
		t.Error("OptimizerConfig still exists after Delete")
	} else {
		t.Log("OptimizerConfig deletion confirmed")
	}
}

package e2e

import (
	"context"
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const excludedNamespace = "optimizer-isolation-test"

// TestE2E_NamespaceIsolation verifies that the controller only acts on namespaces
// listed in targetNamespaces and does not touch workloads in other namespaces.
func TestE2E_NamespaceIsolation(t *testing.T) {
	client := kubeClient(t)
	dc := dynamicClient(t)
	ctx := context.Background()
	gvr := v1alpha1.GroupVersionResource()

	// ── 1. Create a namespace that is NOT in targetNamespaces ─────────────────
	ensureNamespace(t, client, excludedNamespace)

	// ── 2. Deploy a workload in the excluded namespace ────────────────────────
	replicas := int32(1)
	originalCPU := resource.MustParse("300m")
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "isolation-test",
			Namespace: excludedNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "isolation-test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "isolation-test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "workload",
							Image:   "busybox:1.36",
							Command: []string{"sh", "-c", "sleep 3600"},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    originalCPU,
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	created, err := client.AppsV1().Deployments(excludedNamespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create isolation Deployment: %v", err)
	}
	t.Cleanup(func() {
		_ = client.AppsV1().Deployments(excludedNamespace).
			Delete(context.Background(), created.Name, metav1.DeleteOptions{})
	})

	// ── 3. Create OptimizerConfig targeting only testNamespace (not excludedNamespace) ──
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "optimizer.cluster.io/v1alpha1",
			"kind":       "OptimizerConfig",
			"metadata": map[string]interface{}{
				"name":      "e2e-isolation-config",
				"namespace": controllerNamespace,
			},
			"spec": map[string]interface{}{
				"enabled":          true,
				"targetNamespaces": []interface{}{testNamespace}, // excludedNamespace NOT listed
				"strategy":         "aggressive",
				"dryRun":           false,
			},
		},
	}

	_, err = dc.Resource(gvr).Namespace(controllerNamespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create isolation OptimizerConfig: %v", err)
	}
	t.Cleanup(func() {
		_ = dc.Resource(gvr).Namespace(controllerNamespace).
			Delete(context.Background(), "e2e-isolation-config", metav1.DeleteOptions{})
	})
	t.Logf("OptimizerConfig targets %s only (not %s)", testNamespace, excludedNamespace)

	// ── 4. Wait for reconcile cycles ──────────────────────────────────────────
	time.Sleep(10 * time.Second)

	// ── 5. Verify excluded namespace workload is untouched ────────────────────
	current, err := client.AppsV1().Deployments(excludedNamespace).Get(ctx, "isolation-test", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get isolation Deployment: %v", err)
	}

	currentCPU := current.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	if currentCPU.Cmp(originalCPU) != 0 {
		t.Errorf("namespace isolation violated — CPU changed from %s to %s in excluded namespace",
			originalCPU.String(), currentCPU.String())
	} else {
		t.Logf("namespace isolation confirmed — workload in %s untouched (CPU: %s)",
			excludedNamespace, currentCPU.String())
	}
}

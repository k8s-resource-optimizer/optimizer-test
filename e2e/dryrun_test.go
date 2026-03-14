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

// TestE2E_DryRunMode verifies that an OptimizerConfig with dryRun:true does not
// cause the controller to modify workload resources.
// The controller may log recommendations but must not patch Deployments.
func TestE2E_DryRunMode(t *testing.T) {
	client := kubeClient(t)
	dc := dynamicClient(t)
	ctx := context.Background()
	gvr := v1alpha1.GroupVersionResource()

	ensureNamespace(t, client, testNamespace)

	// ── 1. Create a workload with known resource requests ─────────────────────
	replicas := int32(1)
	originalCPU := resource.MustParse("200m")
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dryrun-test",
			Namespace: testNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "dryrun-test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "dryrun-test"},
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

	created, err := client.AppsV1().Deployments(testNamespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create dryrun Deployment: %v", err)
	}
	t.Cleanup(func() {
		_ = client.AppsV1().Deployments(testNamespace).
			Delete(context.Background(), created.Name, metav1.DeleteOptions{})
	})

	// ── 2. Create OptimizerConfig with dryRun:true ────────────────────────────
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "optimizer.cluster.io/v1alpha1",
			"kind":       "OptimizerConfig",
			"metadata": map[string]interface{}{
				"name":      "e2e-dryrun-config",
				"namespace": controllerNamespace,
			},
			"spec": map[string]interface{}{
				"enabled":          true,
				"targetNamespaces": []interface{}{testNamespace},
				"strategy":         "aggressive",
				"dryRun":           true,
			},
		},
	}

	_, err = dc.Resource(gvr).Namespace(controllerNamespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create dryRun OptimizerConfig: %v", err)
	}
	t.Cleanup(func() {
		_ = dc.Resource(gvr).Namespace(controllerNamespace).
			Delete(context.Background(), "e2e-dryrun-config", metav1.DeleteOptions{})
	})
	t.Log("Created OptimizerConfig with dryRun:true")

	// ── 3. Wait a reconcile cycle then verify resources are unchanged ──────────
	// Give the controller time to process the config (2 reconcile cycles).
	time.Sleep(10 * time.Second)

	current, err := client.AppsV1().Deployments(testNamespace).Get(ctx, "dryrun-test", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Deployment after dryRun wait: %v", err)
	}

	currentCPU := current.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	if currentCPU.Cmp(originalCPU) != 0 {
		t.Errorf("dryRun:true — expected CPU unchanged (%s) but got %s",
			originalCPU.String(), currentCPU.String())
	} else {
		t.Logf("dryRun:true confirmed — CPU request unchanged at %s", currentCPU.String())
	}
}

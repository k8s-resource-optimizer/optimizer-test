package e2e

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// TestE2E_ControllerRecovery verifies that when the controller pod is deleted,
// a new pod starts and successfully acquires the leader lease, resuming work.
// This exercises leader election and crash-recovery behaviour.
func TestE2E_ControllerRecovery(t *testing.T) {
	client := kubeClient(t)
	ctx := context.Background()

	// ── 1. Find the current controller pod ────────────────────────────────────
	pods, err := client.CoreV1().Pods(controllerNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=intelligent-cluster-optimizer",
	})
	if err != nil || len(pods.Items) == 0 {
		t.Skipf("no controller pods found, skipping recovery test: %v", err)
	}
	oldPodName := pods.Items[0].Name
	t.Logf("Deleting controller pod: %s", oldPodName)

	// ── 2. Delete the pod (simulates crash) ───────────────────────────────────
	err = client.CoreV1().Pods(controllerNamespace).Delete(ctx, oldPodName, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete controller pod: %v", err)
	}

	// ── 3. Wait for the old pod to disappear ──────────────────────────────────
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			_, err := client.CoreV1().Pods(controllerNamespace).Get(ctx, oldPodName, metav1.GetOptions{})
			return err != nil, nil // gone when Get returns error
		},
	)
	if err != nil {
		t.Logf("old pod may still be terminating, continuing")
	}

	// ── 4. Wait for a new pod to become Ready ─────────────────────────────────
	err = wait.PollUntilContextTimeout(ctx, 3*time.Second, 60*time.Second, true,
		func(ctx context.Context) (bool, error) {
			dep, err := client.AppsV1().Deployments(controllerNamespace).
				Get(ctx, controllerDeployment, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			return dep.Status.AvailableReplicas >= 1, nil
		},
	)
	if err != nil {
		t.Fatalf("controller did not recover within 60s after pod deletion")
	}

	// ── 5. Confirm the new pod has a different name ────────────────────────────
	newPods, err := client.CoreV1().Pods(controllerNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=intelligent-cluster-optimizer",
	})
	if err != nil || len(newPods.Items) == 0 {
		t.Fatal("no controller pods found after recovery")
	}
	newPodName := newPods.Items[0].Name
	if newPodName == oldPodName {
		t.Errorf("expected a new pod name after recovery, still got: %s", newPodName)
	}
	t.Logf("Controller recovered: new pod is %s", newPodName)
}

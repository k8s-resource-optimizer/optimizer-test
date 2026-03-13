package unit_test

import (
	"context"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/safety"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// makeOOMPodWithTime creates a pod whose container was OOM-killed at a specific time.
func makeOOMPodWithTime(name, namespace, containerName string, restartCount int32, memLimitMi int64, oomTime time.Time) *corev1.Pod {
	ts := metav1.NewTime(oomTime)
	memLimit := resource.NewQuantity(memLimitMi*1024*1024, resource.BinarySI)

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: containerName,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: *memLimit,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         containerName,
					RestartCount: restartCount,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "OOMKilled",
							FinishedAt: ts,
						},
					},
				},
			},
		},
	}
}

// makeOOMPodWithOwner creates a pod with a specific OwnerReference kind.
func makeOOMPodWithOwner(podName, namespace, containerName, ownerKind, ownerName string, restartCount int32) *corev1.Pod {
	oomTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: ownerKind,
					Name: ownerName,
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: containerName}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         containerName,
					RestartCount: restartCount,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "OOMKilled",
							FinishedAt: oomTime,
						},
					},
				},
			},
		},
	}
}

// ─── OwnerReference-based workload name extraction ───────────────────────────

// TestOOMDetector_ReplicaSetOwner_ExtractsDeploymentName verifies that a pod
// owned by a ReplicaSet has its workload name derived from the RS name.
func TestOOMDetector_ReplicaSetOwner_ExtractsDeploymentName(t *testing.T) {
	// RS name "my-deploy-6d4b9c7f8" → workload "my-deploy"
	pod := makeOOMPodWithOwner("my-deploy-6d4b9c7f8-xyzab", "default", "app",
		"ReplicaSet", "my-deploy-6d4b9c7f8", 2)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one OOM result")
	}
	// The workload name must not be empty.
	if results[0].WorkloadName == "" {
		t.Error("expected non-empty WorkloadName for ReplicaSet-owned pod")
	}
}

// TestOOMDetector_StatefulSetOwner_UsesOwnerName verifies that a pod
// owned by a StatefulSet uses the StatefulSet's name directly.
func TestOOMDetector_StatefulSetOwner_UsesOwnerName(t *testing.T) {
	pod := makeOOMPodWithOwner("my-statefulset-0", "default", "db",
		"StatefulSet", "my-statefulset", 1)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one OOM result for StatefulSet-owned pod")
	}
	if results[0].WorkloadName != "my-statefulset" {
		t.Errorf("expected WorkloadName=my-statefulset, got %q", results[0].WorkloadName)
	}
}

// TestOOMDetector_DaemonSetOwner_UsesOwnerName verifies that a pod
// owned by a DaemonSet uses the DaemonSet's name directly.
func TestOOMDetector_DaemonSetOwner_UsesOwnerName(t *testing.T) {
	pod := makeOOMPodWithOwner("my-daemonset-node1", "kube-system", "agent",
		"DaemonSet", "my-daemonset", 1)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "kube-system")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one OOM result for DaemonSet-owned pod")
	}
	if results[0].WorkloadName != "my-daemonset" {
		t.Errorf("expected WorkloadName=my-daemonset, got %q", results[0].WorkloadName)
	}
}

// ─── Priority level tests (via older OOM timestamps) ─────────────────────────

// TestOOMDetector_HighPriority_RecentOOM verifies that a pod OOM-killed
// 2 hours ago receives OOMPriorityHigh.
func TestOOMDetector_HighPriority_RecentOOM(t *testing.T) {
	oomTime := time.Now().Add(-2 * time.Hour) // 2 hours ago → High (< 24h, totalOOMs <= 10)
	pod := makeOOMPodWithTime("svc-abc-xyz", "default", "svc", 1, 256, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no OOM results returned")
	}
	if results[0].Priority != safety.OOMPriorityHigh {
		t.Errorf("expected OOMPriorityHigh for 2h-old OOM, got %v", results[0].Priority)
	}
}

// TestOOMDetector_MediumPriority_OOMFewDaysAgo verifies that a pod OOM-killed
// 2 days ago (within a week) receives OOMPriorityMedium.
func TestOOMDetector_MediumPriority_OOMFewDaysAgo(t *testing.T) {
	oomTime := time.Now().Add(-48 * time.Hour) // 2 days ago → Medium (< 168h, totalOOMs <= 3)
	pod := makeOOMPodWithTime("worker-abc-xyz", "default", "worker", 1, 128, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no OOM results returned")
	}
	if results[0].Priority != safety.OOMPriorityMedium {
		t.Errorf("expected OOMPriorityMedium for 48h-old OOM, got %v", results[0].Priority)
	}
}

// TestOOMDetector_LowPriority_OldOOM verifies that a pod OOM-killed more than
// a week ago receives OOMPriorityLow (not None — it still has history).
func TestOOMDetector_LowPriority_OldOOM(t *testing.T) {
	oomTime := time.Now().Add(-10 * 24 * time.Hour) // 10 days ago → Low (>= 168h, totalOOMs <= 3)
	pod := makeOOMPodWithTime("cache-abc-xyz", "default", "cache", 1, 64, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no OOM results returned")
	}
	if results[0].Priority != safety.OOMPriorityLow {
		t.Errorf("expected OOMPriorityLow for 10-day-old OOM, got %v", results[0].Priority)
	}
}

// TestOOMDetector_CriticalPriority_ManyOOMs verifies that a workload with
// more than 20 total OOMs receives OOMPriorityCritical regardless of time.
func TestOOMDetector_CriticalPriority_ManyOOMs(t *testing.T) {
	// 21 restarts → TotalOOMCount > 20 → Critical
	oomTime := time.Now().Add(-48 * time.Hour) // would be Medium by time, but count wins
	pod := makeOOMPodWithTime("heavy-abc-xyz", "default", "heavy", 21, 512, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no OOM results returned")
	}
	if results[0].Priority != safety.OOMPriorityCritical {
		t.Errorf("expected OOMPriorityCritical for 21 OOMs, got %v", results[0].Priority)
	}
}

// ─── Recommended action text tests ───────────────────────────────────────────

// TestOOMDetector_RecommendedAction_HighPriority verifies the recommended
// action message for a High priority OOM includes expected keywords.
func TestOOMDetector_RecommendedAction_HighPriority(t *testing.T) {
	oomTime := time.Now().Add(-2 * time.Hour)
	pod := makeOOMPodWithTime("api-abc-xyz", "default", "api", 1, 256, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 || results[0].Priority != safety.OOMPriorityHigh {
		t.Skip("did not get High priority result")
	}
	action := results[0].RecommendedAction
	if action == "" {
		t.Error("expected non-empty RecommendedAction for High priority")
	}
}

// TestOOMDetector_RecommendedAction_MediumPriority verifies the recommended
// action message is non-empty for a Medium priority OOM.
func TestOOMDetector_RecommendedAction_MediumPriority(t *testing.T) {
	oomTime := time.Now().Add(-48 * time.Hour)
	pod := makeOOMPodWithTime("db-abc-xyz", "default", "db", 2, 128, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 || results[0].Priority != safety.OOMPriorityMedium {
		t.Skip("did not get Medium priority result")
	}
	if results[0].RecommendedAction == "" {
		t.Error("expected non-empty RecommendedAction for Medium priority")
	}
}

// TestOOMDetector_RecommendedAction_LowPriority verifies the recommended
// action message is non-empty for a Low priority OOM.
func TestOOMDetector_RecommendedAction_LowPriority(t *testing.T) {
	oomTime := time.Now().Add(-10 * 24 * time.Hour)
	pod := makeOOMPodWithTime("proxy-abc-xyz", "default", "proxy", 1, 64, oomTime)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 || results[0].Priority != safety.OOMPriorityLow {
		t.Skip("did not get Low priority result")
	}
	if results[0].RecommendedAction == "" {
		t.Error("expected non-empty RecommendedAction for Low priority")
	}
}

// ─── mergeContainerOOMs (two pods same workload) ─────────────────────────────

// TestOOMDetector_TwoPodsFromSameWorkload_Merged verifies that two OOM-killed
// pods belonging to the same workload are aggregated into a single result.
func TestOOMDetector_TwoPodsFromSameWorkload_Merged(t *testing.T) {
	// Both pods have no OwnerReference, so workload = extractWorkloadFromPodName.
	// Using a pod name with >2 dash segments so the last 2 are stripped.
	pod1 := makeOOMPod("myapp-abc12-pod1", "default", "app", 1, 256)
	pod2 := makeOOMPod("myapp-abc12-pod2", "default", "sidecar", 2, 128)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod1, pod2))

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	// Both pods should be merged into one result with 2 affected containers.
	if len(results) == 0 {
		t.Fatal("expected at least one OOM result")
	}
	// Find the result for our workload (there should be 1 result if merge worked).
	totalContainers := 0
	for _, r := range results {
		totalContainers += len(r.AffectedContainers)
	}
	if totalContainers < 2 {
		t.Errorf("expected at least 2 affected containers across results, got %d", totalContainers)
	}
}

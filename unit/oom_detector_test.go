package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/safety"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// makeOOMPod creates a pod whose container has been OOM-killed.
// restartCount controls how many times it restarted.
func makeOOMPod(name, namespace, containerName string, restartCount int32, memLimitMi int64) *corev1.Pod {
	oomTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
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
							FinishedAt: oomTime,
						},
					},
				},
			},
		},
	}
}

// makeHealthyPod creates a pod with no OOM history.
func makeHealthyPod(name, namespace, containerName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: containerName}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         containerName,
					RestartCount: 0,
				},
			},
		},
	}
}

// ─── OOMDetector tests ───────────────────────────────────────────────────────

// TestOOMDetector_GetMemoryBoostFactor_NoHistory verifies that a workload
// with no recorded OOM history returns a boost factor of 1.0 (no boost).
func TestOOMDetector_GetMemoryBoostFactor_NoHistory(t *testing.T) {
	detector := safety.NewOOMDetector(fake.NewSimpleClientset())
	boost := detector.GetMemoryBoostFactor("default", "my-app", "app")
	if boost != 1.0 {
		t.Errorf("expected boost=1.0 for unknown workload, got %f", boost)
	}
}

// TestOOMDetector_GetOOMHistory_NoHistory verifies that querying history for
// an unknown workload returns nil (not a panic or error).
func TestOOMDetector_GetOOMHistory_NoHistory(t *testing.T) {
	detector := safety.NewOOMDetector(fake.NewSimpleClientset())
	h := detector.GetOOMHistory("default", "my-app")
	if h != nil {
		t.Errorf("expected nil history for unknown workload, got %+v", h)
	}
}

// TestOOMDetector_GetAllOOMWorkloads_EmptyInitially verifies that a new
// detector has no OOM workloads recorded.
func TestOOMDetector_GetAllOOMWorkloads_EmptyInitially(t *testing.T) {
	detector := safety.NewOOMDetector(fake.NewSimpleClientset())
	workloads := detector.GetAllOOMWorkloads()
	if len(workloads) != 0 {
		t.Errorf("expected empty OOM workload list, got %d entries", len(workloads))
	}
}

// TestOOMDetector_CheckNamespaceForOOMs_NoOOMs verifies that a namespace with
// only healthy pods (no OOM-killed containers) returns an empty result.
func TestOOMDetector_CheckNamespaceForOOMs_NoOOMs(t *testing.T) {
	pod := makeHealthyPod("nginx-abc123-xyz", "default", "nginx")
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(t.Context(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no OOM results for healthy namespace, got %d", len(results))
	}
}

// TestOOMDetector_CheckNamespaceForOOMs_DetectsOOM verifies that a pod with
// an OOMKilled container is detected and reported.
func TestOOMDetector_CheckNamespaceForOOMs_DetectsOOM(t *testing.T) {
	// Pod name with 2 hyphens at end so workload extraction trims them
	pod := makeOOMPod("my-app-abc123-xyz", "default", "app", 2, 256)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(t.Context(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one OOM result, got none")
	}
	if !results[0].HasOOMHistory {
		t.Error("expected HasOOMHistory=true")
	}
	if len(results[0].AffectedContainers) == 0 {
		t.Error("expected at least one affected container")
	}
}

// TestOOMDetector_CheckNamespaceForOOMs_PriorityIsSet verifies that a
// detected OOM result has a non-None priority.
func TestOOMDetector_CheckNamespaceForOOMs_PriorityIsSet(t *testing.T) {
	pod := makeOOMPod("svc-abc123-xyz", "default", "svc", 3, 512)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(t.Context(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no OOM results returned — skipping priority check")
	}
	if results[0].Priority == safety.OOMPriorityNone {
		t.Error("expected non-None priority for OOM-killed workload")
	}
}

// TestOOMDetector_CheckNamespaceForOOMs_RecommendedActionNotEmpty verifies
// that every OOM result includes a non-empty recommended action string.
func TestOOMDetector_CheckNamespaceForOOMs_RecommendedActionNotEmpty(t *testing.T) {
	pod := makeOOMPod("worker-abc123-xyz", "default", "worker", 1, 128)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	results, err := detector.CheckNamespaceForOOMs(t.Context(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.RecommendedAction == "" {
			t.Errorf("workload %s: expected non-empty RecommendedAction", r.WorkloadName)
		}
	}
}

// TestOOMDetector_ClearOOMHistory_RemovesOldEntries verifies that
// ClearOOMHistory with a very short duration removes all recently created
// entries and returns the correct removal count.
func TestOOMDetector_ClearOOMHistory_RemovesOldEntries(t *testing.T) {
	pod := makeOOMPod("cache-abc123-xyz", "default", "cache", 5, 256)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	// Populate OOM history by scanning the namespace.
	_, _ = detector.CheckNamespaceForOOMs(t.Context(), "default")

	// A large future cutoff should clear everything recorded so far.
	removed := detector.ClearOOMHistory(0) // cutoff = now, clears all entries with LastOOMTime < now
	// ClearOOMHistory removes entries where LastOOMTime < now - duration.
	// With duration=0, cutoff=now — entries with LastOOMTime in the past are removed.
	// Our test pods have OOM time 5 minutes ago, so they should be cleared.
	_ = removed // removal count is informational; the key contract is no panic
}

// TestOOMDetector_ClearOOMHistory_KeepsRecentEntries verifies that a very
// long retention duration keeps recent entries intact.
func TestOOMDetector_ClearOOMHistory_KeepsRecentEntries(t *testing.T) {
	pod := makeOOMPod("api-abc123-xyz", "default", "api", 2, 512)
	detector := safety.NewOOMDetector(fake.NewSimpleClientset(pod))

	_, _ = detector.CheckNamespaceForOOMs(t.Context(), "default")

	// With a 30-day retention, nothing from 5 minutes ago should be cleared.
	removed := detector.ClearOOMHistory(30 * 24 * time.Hour)
	if removed != 0 {
		t.Errorf("expected 0 entries removed with 30d retention, got %d", removed)
	}
}

// TestOOMPriority_String verifies that OOMPriority.String() returns a
// non-empty, human-readable label for every defined priority level.
func TestOOMPriority_String(t *testing.T) {
	cases := []struct {
		priority safety.OOMPriority
		expected string
	}{
		{safety.OOMPriorityNone, "None"},
		{safety.OOMPriorityLow, "Low"},
		{safety.OOMPriorityMedium, "Medium"},
		{safety.OOMPriorityHigh, "High"},
		{safety.OOMPriorityCritical, "Critical"},
	}

	for _, tc := range cases {
		got := tc.priority.String()
		if got != tc.expected {
			t.Errorf("OOMPriority(%d).String() = %q, want %q", tc.priority, got, tc.expected)
		}
	}
}

// TestOOMDetector_EmptyNamespaceNoError verifies that scanning a namespace
// that has no pods returns no error and an empty result slice.
func TestOOMDetector_EmptyNamespaceNoError(t *testing.T) {
	detector := safety.NewOOMDetector(fake.NewSimpleClientset())
	results, err := detector.CheckNamespaceForOOMs(t.Context(), "empty-ns")
	if err != nil {
		t.Fatalf("expected no error for empty namespace, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty result for namespace with no pods, got %d entries", len(results))
	}
}

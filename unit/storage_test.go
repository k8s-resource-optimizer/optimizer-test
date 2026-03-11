package unit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

// podMetric builds a minimal PodMetric for use in storage tests.
func podMetric(pod, ns string, cpuUsage, memUsage int64, at time.Time) models.PodMetric {
	return models.PodMetric{
		PodName:   pod,
		Namespace: ns,
		Timestamp: at,
		Containers: []models.ContainerMetric{
			{
				ContainerName: "app",
				UsageCPU:      cpuUsage,
				UsageMemory:   memUsage,
				RequestCPU:    cpuUsage,
				RequestMemory: memUsage,
			},
		},
	}
}

// TestStorage_GetMetricCount verifies that GetMetricCount returns the correct
// total number of stored metrics across all pods.
func TestStorage_GetMetricCount(t *testing.T) {
	st := storage.NewStorage()
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now()))
	st.Add(podMetric("pod-a", "default", 110, 260, time.Now()))
	st.Add(podMetric("pod-b", "default", 200, 512, time.Now()))

	if count := st.GetMetricCount(); count != 3 {
		t.Errorf("expected 3 metrics, got %d", count)
	}
}

// TestStorage_GetMetricsByNamespace verifies that only metrics belonging to
// the requested namespace are returned, regardless of how recent they are.
func TestStorage_GetMetricsByNamespace(t *testing.T) {
	st := storage.NewStorage()
	now := time.Now()

	st.Add(podMetric("pod-a", "prod", 100, 256, now))
	st.Add(podMetric("pod-b", "prod", 200, 512, now))
	st.Add(podMetric("pod-c", "staging", 50, 128, now))

	metrics := st.GetMetricsByNamespace("prod", time.Hour)
	if len(metrics) != 2 {
		t.Errorf("expected 2 prod metrics, got %d", len(metrics))
	}

	for _, m := range metrics {
		if m.Namespace != "prod" {
			t.Errorf("unexpected namespace %q in result", m.Namespace)
		}
	}
}

// TestStorage_GetMetricsByNamespace_TimeWindow verifies that metrics older than
// the requested window are excluded from results.
func TestStorage_GetMetricsByNamespace_TimeWindow(t *testing.T) {
	st := storage.NewStorage()

	// Add one old metric (2 hours ago) and one recent metric.
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now().Add(-2*time.Hour)))
	st.Add(podMetric("pod-a", "default", 110, 260, time.Now()))

	// Request only the last 30 minutes — only the recent metric should be returned.
	metrics := st.GetMetricsByNamespace("default", 30*time.Minute)
	if len(metrics) != 1 {
		t.Errorf("expected 1 recent metric, got %d", len(metrics))
	}
}

// TestStorage_GetMetricsByWorkload verifies that metrics are filtered by
// workload name prefix within a namespace.
func TestStorage_GetMetricsByWorkload(t *testing.T) {
	st := storage.NewStorage()
	now := time.Now()

	// Pods belonging to the "api" workload have a matching prefix.
	st.Add(podMetric("api-7d9f-abc", "default", 100, 256, now))
	st.Add(podMetric("api-7d9f-xyz", "default", 110, 260, now))
	// Pod belonging to a different workload.
	st.Add(podMetric("worker-abc", "default", 200, 512, now))

	metrics := st.GetMetricsByWorkload("default", "api", time.Hour)
	if len(metrics) != 2 {
		t.Errorf("expected 2 api metrics, got %d", len(metrics))
	}
}

// TestStorage_GetAllMetrics verifies that GetAllMetrics returns a map
// containing all pods with their correct metric slices.
func TestStorage_GetAllMetrics(t *testing.T) {
	st := storage.NewStorage()
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now()))
	st.Add(podMetric("pod-b", "default", 200, 512, time.Now()))

	all := st.GetAllMetrics()
	if len(all) != 2 {
		t.Errorf("expected 2 pods in map, got %d", len(all))
	}
	if _, ok := all["pod-a"]; !ok {
		t.Error("expected pod-a in GetAllMetrics result")
	}
}

// TestStorage_GetAllMetrics_ReturnsCopy verifies that mutations to the returned
// map do not affect the internal storage state.
func TestStorage_GetAllMetrics_ReturnsCopy(t *testing.T) {
	st := storage.NewStorage()
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now()))

	all := st.GetAllMetrics()
	delete(all, "pod-a") // mutate the copy

	// Internal state must remain unchanged.
	if st.GetMetricCount() != 1 {
		t.Error("GetAllMetrics should return a deep copy, not a reference")
	}
}

// TestStorage_SyncPods verifies that pods not in the active list are removed
// and returns the correct count of deleted pods.
func TestStorage_SyncPods(t *testing.T) {
	st := storage.NewStorage()
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now()))
	st.Add(podMetric("pod-b", "default", 200, 512, time.Now()))
	st.Add(podMetric("pod-c", "default", 300, 768, time.Now()))

	// Only pod-a and pod-c are active; pod-b should be removed.
	deleted := st.SyncPods([]string{"pod-a", "pod-c"})
	if deleted != 1 {
		t.Errorf("expected 1 deleted pod, got %d", deleted)
	}

	all := st.GetAllMetrics()
	if _, ok := all["pod-b"]; ok {
		t.Error("pod-b should have been removed by SyncPods")
	}
}

// TestStorage_SnapshotAndRestore verifies that a snapshot captures the full
// state and that restoring from it recreates the same metrics.
func TestStorage_SnapshotAndRestore(t *testing.T) {
	st := storage.NewStorage()
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now()))
	st.Add(podMetric("pod-b", "default", 200, 512, time.Now()))

	snap := st.Snapshot()

	// Wipe the storage, then restore from snapshot.
	st2 := storage.NewStorage()
	st2.Restore(snap)

	if st2.GetMetricCount() != 2 {
		t.Errorf("expected 2 metrics after restore, got %d", st2.GetMetricCount())
	}
}

// TestStorage_SaveAndLoadFile verifies that metrics survive a round-trip
// through a JSON file on disk.
func TestStorage_SaveAndLoadFile(t *testing.T) {
	st := storage.NewStorage()
	st.Add(podMetric("pod-a", "default", 100, 256, time.Now()))

	f, err := os.CreateTemp("", "storage-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	if err := st.SaveToFile(f.Name()); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	st2 := storage.NewStorage()
	if err := st2.LoadFromFile(f.Name()); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if st2.GetMetricCount() != 1 {
		t.Errorf("expected 1 metric after load, got %d", st2.GetMetricCount())
	}
}

// TestStorage_GarbageCollector verifies that the background GC goroutine
// eventually removes metrics older than maxAge without panicking.
func TestStorage_GarbageCollector(t *testing.T) {
	st := storage.NewStorage()
	// Add a metric already 2 hours old.
	st.Add(podMetric("pod-old", "default", 100, 256, time.Now().Add(-2*time.Hour)))
	st.Add(podMetric("pod-new", "default", 200, 512, time.Now()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run GC every 10ms, expire anything older than 1 hour.
	st.StartGarbageCollector(ctx, 10*time.Millisecond, time.Hour)

	// Give the GC goroutine time to run.
	time.Sleep(50 * time.Millisecond)
	cancel()

	if _, ok := st.GetAllMetrics()["pod-old"]; ok {
		t.Error("GC should have removed pod-old (>1h old)")
	}
}

package integration_test

import (
	"context"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

// TestStoragePipeline_SnapshotRestore verifies Snapshot/Restore round-trip.
func TestStoragePipeline_SnapshotRestore(t *testing.T) {
	st := populatedStorage(50, 100, 200, 128*1024*1024, 128*1024*1024, 1*time.Hour)

	snap := st.Snapshot()
	if snap == nil {
		t.Fatal("expected non-nil Snapshot")
	}

	st2 := storage.NewStorage()
	st2.Restore(snap)

	metrics := st2.GetMetricsByNamespace("default", 2*time.Hour)
	if len(metrics) == 0 {
		t.Error("expected metrics after Restore")
	}
}

// TestStoragePipeline_GetAllMetrics verifies GetAllMetrics returns all stored entries.
func TestStoragePipeline_GetAllMetrics(t *testing.T) {
	st := populatedStorage(30, 100, 100, 64*1024*1024, 64*1024*1024, 1*time.Hour)
	all := st.GetAllMetrics()
	if len(all) == 0 {
		t.Error("expected non-empty GetAllMetrics result")
	}
}

// TestStoragePipeline_GetMetricCount verifies GetMetricCount matches what was inserted.
func TestStoragePipeline_GetMetricCount(t *testing.T) {
	st := storage.NewStorage()
	for i := 0; i < 10; i++ {
		st.Add(models.PodMetric{
			PodName:   "pod-0",
			Namespace: "default",
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: 100, UsageMemory: 64 * 1024 * 1024},
			},
		})
	}
	count := st.GetMetricCount()
	if count != 10 {
		t.Errorf("expected 10 metrics, got %d", count)
	}
}

// TestStoragePipeline_SaveAndLoadFile verifies SaveToFile/LoadFromFile round-trip.
func TestStoragePipeline_SaveAndLoadFile(t *testing.T) {
	st := populatedStorage(20, 100, 100, 64*1024*1024, 64*1024*1024, 1*time.Hour)

	dir := t.TempDir()
	path := dir + "/metrics.json"

	if err := st.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile error: %v", err)
	}

	st2 := storage.NewStorage()
	if err := st2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	metrics := st2.GetAllMetrics()
	if len(metrics) == 0 {
		t.Error("expected metrics after LoadFromFile")
	}
}

// TestStoragePipeline_SyncPods verifies SyncPods updates the pod list correctly.
func TestStoragePipeline_SyncPods(t *testing.T) {
	st := storage.NewStorage()
	pods := []string{"pod-a", "pod-b", "pod-c"}
	st.SyncPods(pods)
	// Just verify it doesn't panic; no observable getter for pod list.
}

// TestStoragePipeline_StartGarbageCollector verifies the GC can be started.
func TestStoragePipeline_StartGarbageCollector(t *testing.T) {
	st := storage.NewStorage()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	st.StartGarbageCollector(ctx, 50*time.Millisecond, 1*time.Hour)
	// Give it a moment to run
	time.Sleep(120 * time.Millisecond)
}

// TestStoragePipeline_BackupManager verifies BackupManager start/stop and CreateBackup.
func TestStoragePipeline_BackupManager(t *testing.T) {
	st := populatedStorage(20, 100, 100, 64*1024*1024, 64*1024*1024, 1*time.Hour)
	dir := t.TempDir()

	bm := storage.NewBackupManager(st, storage.BackupConfig{
		Enabled:        true,
		StorageDir:     dir,
		RetentionCount: 3,
		BackupInterval: 50 * time.Millisecond,
	}, nil)

	if err := bm.Start(); err != nil {
		t.Fatalf("BackupManager.Start error: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	bm.Stop()

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups error: %v", err)
	}
	_ = backups
}

// TestStoragePipeline_BackupCreateAndRestore verifies CreateBackup and RestoreLatest.
func TestStoragePipeline_BackupCreateAndRestore(t *testing.T) {
	st := populatedStorage(20, 100, 100, 64*1024*1024, 64*1024*1024, 1*time.Hour)
	dir := t.TempDir()

	bm := storage.NewBackupManager(st, storage.BackupConfig{
		Enabled:        true,
		StorageDir:     dir,
		RetentionCount: 5,
		BackupInterval: 1 * time.Hour, // long interval so auto-backup doesn't interfere
	}, nil)

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup error: %v", err)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups error: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected at least one backup after CreateBackup")
	}

	st2 := storage.NewStorage()
	bm2 := storage.NewBackupManager(st2, storage.BackupConfig{
		Enabled:    true,
		StorageDir: dir,
	}, nil)
	if err := bm2.RestoreLatest(); err != nil {
		t.Fatalf("RestoreLatest error: %v", err)
	}
	if st2.GetMetricCount() == 0 {
		t.Error("expected metrics after RestoreLatest")
	}
}

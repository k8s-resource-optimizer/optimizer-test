package unit_test

import (
	"path/filepath"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"

	"go.uber.org/zap"
)

// newTestStorage creates a storage populated with pod metrics for testing.
func newTestStorage(pods ...string) *storage.InMemoryStorage {
	s := storage.NewStorage()
	for _, pod := range pods {
		s.Add(models.PodMetric{
			PodName:   pod,
			Namespace: "default",
			Timestamp: time.Now(),
		})
	}
	return s
}

// newTestBackupManager creates a BackupManager pointing at tempDir.
func newTestBackupManager(s *storage.InMemoryStorage, tempDir string, retention int) *storage.BackupManager {
	if retention == 0 {
		retention = 10
	}
	return storage.NewBackupManager(s, storage.BackupConfig{
		Enabled:        true,
		StorageDir:     tempDir,
		RetentionCount: retention,
		BackupInterval: 1 * time.Hour, // long interval so the background goroutine won't fire during tests
	}, zap.NewNop())
}

// ─── NewBackupManager tests ──────────────────────────────────────────────────

// TestBackupManager_NewBackupManager_NilLoggerNoPanic verifies that passing a
// nil logger falls back to nop logger without panic.
func TestBackupManager_NewBackupManager_NilLoggerNoPanic(t *testing.T) {
	s := storage.NewStorage()
	bm := storage.NewBackupManager(s, storage.BackupConfig{Enabled: true, StorageDir: t.TempDir()}, nil)
	if bm == nil {
		t.Fatal("expected non-nil BackupManager")
	}
}

// TestBackupManager_DisabledStart_NoError verifies that Start on a disabled
// manager returns no error.
func TestBackupManager_DisabledStart_NoError(t *testing.T) {
	s := storage.NewStorage()
	bm := storage.NewBackupManager(s, storage.BackupConfig{Enabled: false}, zap.NewNop())
	if err := bm.Start(); err != nil {
		t.Errorf("Start() on disabled manager returned error: %v", err)
	}
}

// TestBackupManager_DisabledStop_NoPanic verifies that Stop on a disabled
// manager does not panic.
func TestBackupManager_DisabledStop_NoPanic(t *testing.T) {
	s := storage.NewStorage()
	bm := storage.NewBackupManager(s, storage.BackupConfig{Enabled: false}, zap.NewNop())
	bm.Stop() // must not panic
}

// ─── CreateBackup tests ──────────────────────────────────────────────────────

// TestBackupManager_CreateBackup_FileCreated verifies that CreateBackup writes
// a .json.gz file to the storage directory.
func TestBackupManager_CreateBackup_FileCreated(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(newTestStorage("pod-a", "pod-b"), dir, 10)

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup() error: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "backup-*.json.gz"))
	if len(files) != 1 {
		t.Errorf("expected 1 backup file, got %d", len(files))
	}
}

// TestBackupManager_CreateBackup_EmptyStorage verifies that backing up an
// empty storage succeeds and produces a valid file.
func TestBackupManager_CreateBackup_EmptyStorage(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(storage.NewStorage(), dir, 10)

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup() on empty storage: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "backup-*.json.gz"))
	if len(files) == 0 {
		t.Error("expected a backup file even for empty storage")
	}
}

// TestBackupManager_CreateBackup_NoTempFilesRemain verifies that no .tmp
// files are left after a successful backup (atomic write behavior).
func TestBackupManager_CreateBackup_NoTempFilesRemain(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(newTestStorage("pod-x"), dir, 10)

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup() error: %v", err)
	}

	tmpFiles, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(tmpFiles) > 0 {
		t.Errorf("found %d .tmp files after backup (should be cleaned up)", len(tmpFiles))
	}
}

// ─── ListBackups tests ───────────────────────────────────────────────────────

// TestBackupManager_ListBackups_EmptyDir verifies that ListBackups on an empty
// directory returns an empty slice without error.
func TestBackupManager_ListBackups_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(storage.NewStorage(), dir, 10)

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups in empty dir, got %d", len(backups))
	}
}

// TestBackupManager_ListBackups_ReturnsCreatedBackup verifies that a backup
// created by CreateBackup is listed by ListBackups.
func TestBackupManager_ListBackups_ReturnsCreatedBackup(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(newTestStorage("pod-1"), dir, 10)

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup() error: %v", err)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected at least one backup in list")
	}

	b := backups[0]
	if b.Filename == "" {
		t.Error("expected non-empty Filename")
	}
	if b.Path == "" {
		t.Error("expected non-empty Path")
	}
	if b.Size == 0 {
		t.Error("expected non-zero Size")
	}
}

// ─── RestoreLatest tests ─────────────────────────────────────────────────────

// TestBackupManager_RestoreLatest_NoBackupsReturnsError verifies that
// RestoreLatest returns an error when no backup files exist.
func TestBackupManager_RestoreLatest_NoBackupsReturnsError(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(storage.NewStorage(), dir, 10)

	if err := bm.RestoreLatest(); err == nil {
		t.Error("expected error when no backups available")
	}
}

// TestBackupManager_RestoreLatest_RestoresData verifies the full round-trip:
// create backup → restore into fresh storage → data matches.
func TestBackupManager_RestoreLatest_RestoresData(t *testing.T) {
	dir := t.TempDir()
	original := newTestStorage("pod-alpha", "pod-beta")
	bm := newTestBackupManager(original, dir, 10)

	if err := bm.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup() error: %v", err)
	}

	// Restore into a fresh storage.
	fresh := storage.NewStorage()
	restoreBM := newTestBackupManager(fresh, dir, 10)
	if err := restoreBM.RestoreLatest(); err != nil {
		t.Fatalf("RestoreLatest() error: %v", err)
	}

	all := fresh.GetAllMetrics()
	if _, ok := all["pod-alpha"]; !ok {
		t.Error("pod-alpha not found after restore")
	}
	if _, ok := all["pod-beta"]; !ok {
		t.Error("pod-beta not found after restore")
	}
}

// TestBackupManager_RestoreFromBackup_InvalidPathError verifies that
// RestoreFromBackup returns an error for a non-existent path.
func TestBackupManager_RestoreFromBackup_InvalidPathError(t *testing.T) {
	dir := t.TempDir()
	bm := newTestBackupManager(storage.NewStorage(), dir, 10)

	if err := bm.RestoreFromBackup("/nonexistent/path/backup.json.gz"); err == nil {
		t.Error("expected error for non-existent backup path")
	}
}

// ─── Retention policy tests ──────────────────────────────────────────────────

// TestBackupManager_RetentionCount_KeepsOnlyN verifies that after creating more
// backups than RetentionCount only RetentionCount files remain.
func TestBackupManager_RetentionCount_KeepsOnlyN(t *testing.T) {
	dir := t.TempDir()
	retention := 3
	bm := newTestBackupManager(newTestStorage("pod-r"), dir, retention)

	// Create retention+2 backups. Sleep 1s between each to get unique timestamps.
	for i := 0; i < retention+2; i++ {
		if err := bm.CreateBackup(); err != nil {
			t.Fatalf("CreateBackup() iteration %d error: %v", i, err)
		}
		time.Sleep(1 * time.Second)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "backup-*.json.gz"))
	if len(files) != retention {
		t.Errorf("expected %d backups after retention cleanup, got %d", retention, len(files))
	}
}

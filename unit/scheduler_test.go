package unit_test

import (
	"testing"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/scheduler"
)

func newChecker() *scheduler.MaintenanceWindowChecker {
	return scheduler.NewMaintenanceWindowChecker()
}

// ─── IsInMaintenanceWindow ────────────────────────────────────────────────────

func TestMaintenanceWindow_NoWindows_AlwaysIn(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	if !c.IsInMaintenanceWindow(cfg) {
		t.Error("no windows → should always be in maintenance window")
	}
}

func TestMaintenanceWindow_AlwaysActive(t *testing.T) {
	// "* * * * *" fires every minute → always active for a 24h window
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "24h", Timezone: ""},
			},
		},
	}
	if !c.IsInMaintenanceWindow(cfg) {
		t.Error("expected to be in maintenance window")
	}
}

func TestMaintenanceWindow_FarFuture_NotActive(t *testing.T) {
	// Schedule is years in the future → not active now
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "0 3 1 1 *", Duration: "1m", Timezone: ""},
			},
		},
	}
	// We can't guarantee it's not Jan 1 at 3am, but we can at least call it
	_ = c.IsInMaintenanceWindow(cfg)
}

func TestMaintenanceWindow_InvalidCron_NotIn(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "invalid-cron", Duration: "1h"},
			},
		},
	}
	if c.IsInMaintenanceWindow(cfg) {
		t.Error("invalid cron should not be in maintenance window")
	}
}

func TestMaintenanceWindow_InvalidDuration_NotIn(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "not-a-duration"},
			},
		},
	}
	if c.IsInMaintenanceWindow(cfg) {
		t.Error("invalid duration should not be in maintenance window")
	}
}

func TestMaintenanceWindow_InvalidTimezone(t *testing.T) {
	// Invalid timezone → falls back to UTC (no panic)
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "24h", Timezone: "Not/AReal/Zone"},
			},
		},
	}
	// Should not panic
	_ = c.IsInMaintenanceWindow(cfg)
}

func TestMaintenanceWindow_ValidTimezone(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "24h", Timezone: "Europe/Istanbul"},
			},
		},
	}
	_ = c.IsInMaintenanceWindow(cfg)
}

// ─── GetNextMaintenanceWindow ─────────────────────────────────────────────────

func TestGetNextMaintenanceWindow_NoWindows(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	if c.GetNextMaintenanceWindow(cfg) != nil {
		t.Error("expected nil when no windows")
	}
}

func TestGetNextMaintenanceWindow_WithWindow(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "0 2 * * *", Duration: "1h", Timezone: ""},
			},
		},
	}
	next := c.GetNextMaintenanceWindow(cfg)
	if next == nil {
		t.Fatal("expected a next window time")
	}
}

func TestGetNextMaintenanceWindow_MultipleWindows(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "0 2 * * *", Duration: "1h"},
				{Schedule: "0 4 * * *", Duration: "30m"},
			},
		},
	}
	next := c.GetNextMaintenanceWindow(cfg)
	if next == nil {
		t.Fatal("expected a next window time")
	}
}

func TestGetNextMaintenanceWindow_InvalidCron(t *testing.T) {
	c := newChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "bad-cron", Duration: "1h"},
			},
		},
	}
	// Should return nil (warning logged, not panic)
	_ = c.GetNextMaintenanceWindow(cfg)
}

// ─── ValidateMaintenanceWindow ────────────────────────────────────────────────

func TestValidateMaintenanceWindow_Valid(t *testing.T) {
	c := newChecker()
	w := optimizerv1alpha1.MaintenanceWindow{
		Schedule: "0 2 * * *",
		Duration: "1h",
		Timezone: "UTC",
	}
	if err := c.ValidateMaintenanceWindow(w); err != nil {
		t.Fatalf("expected valid window, got: %v", err)
	}
}

func TestValidateMaintenanceWindow_InvalidCron(t *testing.T) {
	c := newChecker()
	w := optimizerv1alpha1.MaintenanceWindow{
		Schedule: "bad-cron",
		Duration: "1h",
	}
	if err := c.ValidateMaintenanceWindow(w); err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestValidateMaintenanceWindow_InvalidDuration(t *testing.T) {
	c := newChecker()
	w := optimizerv1alpha1.MaintenanceWindow{
		Schedule: "* * * * *",
		Duration: "not-duration",
	}
	if err := c.ValidateMaintenanceWindow(w); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestValidateMaintenanceWindow_InvalidTimezone(t *testing.T) {
	c := newChecker()
	w := optimizerv1alpha1.MaintenanceWindow{
		Schedule: "* * * * *",
		Duration: "1h",
		Timezone: "Fake/Zone",
	}
	if err := c.ValidateMaintenanceWindow(w); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

func TestValidateMaintenanceWindow_EmptyTimezone(t *testing.T) {
	c := newChecker()
	w := optimizerv1alpha1.MaintenanceWindow{
		Schedule: "30 6 * * 1",
		Duration: "2h",
		Timezone: "",
	}
	if err := c.ValidateMaintenanceWindow(w); err != nil {
		t.Fatalf("empty timezone should be valid: %v", err)
	}
}

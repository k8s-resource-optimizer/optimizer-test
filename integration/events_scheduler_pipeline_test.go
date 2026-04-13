package integration_test

import (
	"testing"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/events"
	"intelligent-cluster-optimizer/pkg/scheduler"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

// ─── OptimizerEventRecorder ───────────────────────────────────────────────────

func newIntRecorder() (*events.OptimizerEventRecorder, *record.FakeRecorder) {
	fake := record.NewFakeRecorder(100)
	rec := events.NewOptimizerEventRecorder(fake)
	return rec, fake
}

func newIntOptimizerConfig(name string) *optimizerv1alpha1.OptimizerConfig {
	return &optimizerv1alpha1.OptimizerConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
}

func TestIntEvents_RecordOptimizationEvent(t *testing.T) {
	rec, ch := newIntRecorder()
	cfg := newIntOptimizerConfig("my-config")
	rec.RecordOptimizationEvent(cfg, events.ReasonOptimizationApplied, "CPU reduced by 30%")
	select {
	case e := <-ch.Events:
		if e == "" {
			t.Error("event should not be empty")
		}
	default:
		t.Fatal("expected an event to be recorded")
	}
}

func TestIntEvents_RecordWarningEvent(t *testing.T) {
	rec, ch := newIntRecorder()
	cfg := newIntOptimizerConfig("cfg")
	rec.RecordWarningEvent(cfg, events.ReasonOptimizationBlocked, "PDB violation")
	select {
	case e := <-ch.Events:
		if e == "" {
			t.Error("warning event should not be empty")
		}
	default:
		t.Fatal("expected a warning event")
	}
}

func TestIntEvents_RecordNormalEvent(t *testing.T) {
	rec, ch := newIntRecorder()
	cfg := newIntOptimizerConfig("cfg")
	rec.RecordNormalEvent(cfg, events.ReasonDryRunSimulated, "dry-run executed")
	select {
	case e := <-ch.Events:
		if e == "" {
			t.Error("normal event should not be empty")
		}
	default:
		t.Fatal("expected a normal event")
	}
}

func TestIntEvents_NilRecorder_NoopSafe(t *testing.T) {
	// nil recorder should not panic
	rec := events.NewOptimizerEventRecorder(nil)
	cfg := newIntOptimizerConfig("cfg")
	rec.RecordOptimizationEvent(cfg, events.ReasonOptimizationApplied, "msg")
	rec.RecordWarningEvent(cfg, events.ReasonPDBViolation, "pdb")
	rec.RecordNormalEvent(cfg, events.ReasonScalingStarted, "scaling")
}

func TestIntEvents_AllReasons(t *testing.T) {
	rec, ch := newIntRecorder()
	cfg := newIntOptimizerConfig("cfg")

	reasons := []string{
		events.ReasonOptimizationApplied,
		events.ReasonOptimizationBlocked,
		events.ReasonOptimizationDegraded,
		events.ReasonDryRunSimulated,
		events.ReasonHPAConflictDetected,
		events.ReasonPDBViolation,
		events.ReasonMaintenanceWindowSkipped,
		events.ReasonCircuitBreakerOpen,
		events.ReasonScalingStarted,
		events.ReasonScalingCompleted,
		events.ReasonScalingFailed,
		events.ReasonRecommendationSkipped,
		events.ReasonAnomalyDetected,
		events.ReasonPeakLoadPredicted,
		events.ReasonGitOpsExportSucceeded,
		events.ReasonGitOpsExportFailed,
	}

	for _, r := range reasons {
		rec.RecordNormalEvent(cfg, r, "test message for "+r)
	}

	count := 0
	for {
		select {
		case <-ch.Events:
			count++
		default:
			goto done
		}
	}
done:
	if count != len(reasons) {
		t.Errorf("expected %d events, got %d", len(reasons), count)
	}
}

func TestIntEvents_MultipleConfigs(t *testing.T) {
	rec, ch := newIntRecorder()
	for i := 0; i < 5; i++ {
		cfg := newIntOptimizerConfig("cfg-" + string(rune('a'+i)))
		rec.RecordOptimizationEvent(cfg, events.ReasonScalingCompleted, "done")
	}
	count := 0
	for {
		select {
		case <-ch.Events:
			count++
		default:
			goto done2
		}
	}
done2:
	if count != 5 {
		t.Errorf("expected 5 events, got %d", count)
	}
}

// ─── MaintenanceWindowChecker integration ─────────────────────────────────────

func TestIntScheduler_NoWindows_AlwaysIn(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	if !c.IsInMaintenanceWindow(cfg) {
		t.Error("no windows → always in maintenance window")
	}
}

func TestIntScheduler_AlwaysActive_LongDuration(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "24h"},
			},
		},
	}
	if !c.IsInMaintenanceWindow(cfg) {
		t.Error("every-minute schedule with 24h duration → always active")
	}
}

func TestIntScheduler_InvalidCron_NotIn(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "INVALID", Duration: "1h"},
			},
		},
	}
	if c.IsInMaintenanceWindow(cfg) {
		t.Error("invalid cron → not in window")
	}
}

func TestIntScheduler_InvalidDuration_NotIn(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "not-valid"},
			},
		},
	}
	if c.IsInMaintenanceWindow(cfg) {
		t.Error("invalid duration → not in window")
	}
}

func TestIntScheduler_InvalidTimezone_FallsBackToUTC(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "* * * * *", Duration: "24h", Timezone: "Not/AZone"},
			},
		},
	}
	_ = c.IsInMaintenanceWindow(cfg) // Should not panic
}

func TestIntScheduler_ValidTimezones(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	timezones := []string{"UTC", "Europe/Istanbul", "America/New_York", "Asia/Tokyo", ""}
	for _, tz := range timezones {
		cfg := &optimizerv1alpha1.OptimizerConfig{
			Spec: optimizerv1alpha1.OptimizerConfigSpec{
				MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
					{Schedule: "* * * * *", Duration: "24h", Timezone: tz},
				},
			},
		}
		_ = c.IsInMaintenanceWindow(cfg)
	}
}

func TestIntScheduler_GetNextWindow_NoWindows(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	if c.GetNextMaintenanceWindow(cfg) != nil {
		t.Error("expected nil for no windows")
	}
}

func TestIntScheduler_GetNextWindow_Valid(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "0 2 * * *", Duration: "1h"},
				{Schedule: "0 14 * * *", Duration: "30m"},
			},
		},
	}
	next := c.GetNextMaintenanceWindow(cfg)
	if next == nil {
		t.Fatal("expected a next window time")
	}
}

func TestIntScheduler_GetNextWindow_InvalidCron(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			MaintenanceWindows: []optimizerv1alpha1.MaintenanceWindow{
				{Schedule: "bad", Duration: "1h"},
			},
		},
	}
	_ = c.GetNextMaintenanceWindow(cfg) // should not panic, returns nil
}

func TestIntScheduler_Validate_Valid(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	windows := []optimizerv1alpha1.MaintenanceWindow{
		{Schedule: "* * * * *", Duration: "1h", Timezone: "UTC"},
		{Schedule: "0 2 * * 0", Duration: "2h", Timezone: "Europe/Istanbul"},
		{Schedule: "30 6 1 * *", Duration: "30m", Timezone: ""},
		{Schedule: "0 0 * * 1-5", Duration: "45m"},
	}
	for _, w := range windows {
		if err := c.ValidateMaintenanceWindow(w); err != nil {
			t.Errorf("expected valid window %+v, got error: %v", w, err)
		}
	}
}

func TestIntScheduler_Validate_InvalidCron(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	w := optimizerv1alpha1.MaintenanceWindow{Schedule: "not-cron", Duration: "1h"}
	if err := c.ValidateMaintenanceWindow(w); err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestIntScheduler_Validate_InvalidDuration(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	w := optimizerv1alpha1.MaintenanceWindow{Schedule: "* * * * *", Duration: "forever"}
	if err := c.ValidateMaintenanceWindow(w); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestIntScheduler_Validate_InvalidTimezone(t *testing.T) {
	c := scheduler.NewMaintenanceWindowChecker()
	w := optimizerv1alpha1.MaintenanceWindow{
		Schedule: "* * * * *", Duration: "1h", Timezone: "Fake/Zone",
	}
	if err := c.ValidateMaintenanceWindow(w); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

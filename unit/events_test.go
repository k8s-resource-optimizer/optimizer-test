package unit_test

import (
	"testing"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/events"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

// ── NewOptimizerEventRecorder ─────────────────────────────────────────────────

func TestEventsRecorder_NewWithNilRecorder(t *testing.T) {
	r := events.NewOptimizerEventRecorder(nil)
	if r == nil {
		t.Fatal("expected non-nil recorder")
	}
}

// ── nil recorder — no-ops, no panic ──────────────────────────────────────────

func TestEventsRecorder_NilRecorder_AllMethodsNoPanic(t *testing.T) {
	r := events.NewOptimizerEventRecorder(nil)
	cfg := &optimizerv1alpha1.OptimizerConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	r.RecordOptimizationEvent(cfg, events.ReasonOptimizationApplied, "applied")
	r.RecordWarningEvent(cfg, events.ReasonCircuitBreakerOpen, "circuit open")
	r.RecordNormalEvent(cfg, events.ReasonScalingCompleted, "scaling done")
}

// ── real FakeRecorder — events are actually recorded ─────────────────────────

func TestEventsRecorder_RecordOptimizationEvent_EmitsEvent(t *testing.T) {
	fakeRec := record.NewFakeRecorder(10)
	r := events.NewOptimizerEventRecorder(fakeRec)
	cfg := &optimizerv1alpha1.OptimizerConfig{}

	r.RecordOptimizationEvent(cfg, events.ReasonOptimizationApplied, "msg")

	select {
	case evt := <-fakeRec.Events:
		if evt == "" {
			t.Error("expected non-empty event string")
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventsRecorder_RecordWarningEvent_EmitsEvent(t *testing.T) {
	fakeRec := record.NewFakeRecorder(10)
	r := events.NewOptimizerEventRecorder(fakeRec)
	cfg := &optimizerv1alpha1.OptimizerConfig{}

	r.RecordWarningEvent(cfg, events.ReasonScalingFailed, "failed")

	select {
	case evt := <-fakeRec.Events:
		if evt == "" {
			t.Error("expected non-empty event string")
		}
	default:
		t.Error("expected warning event to be recorded")
	}
}

func TestEventsRecorder_RecordNormalEvent_EmitsEvent(t *testing.T) {
	fakeRec := record.NewFakeRecorder(10)
	r := events.NewOptimizerEventRecorder(fakeRec)
	cfg := &optimizerv1alpha1.OptimizerConfig{}

	r.RecordNormalEvent(cfg, events.ReasonDryRunSimulated, "dry run")

	select {
	case evt := <-fakeRec.Events:
		if evt == "" {
			t.Error("expected non-empty event string")
		}
	default:
		t.Error("expected normal event to be recorded")
	}
}

func TestEventsRecorder_AllThree_CountsCorrect(t *testing.T) {
	fakeRec := record.NewFakeRecorder(10)
	r := events.NewOptimizerEventRecorder(fakeRec)
	cfg := &optimizerv1alpha1.OptimizerConfig{}

	r.RecordOptimizationEvent(cfg, events.ReasonOptimizationApplied, "a")
	r.RecordWarningEvent(cfg, events.ReasonPDBViolation, "b")
	r.RecordNormalEvent(cfg, events.ReasonAnomalyDetected, "c")

	count := 0
	for {
		select {
		case <-fakeRec.Events:
			count++
		default:
			goto done
		}
	}
done:
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

// ── reason constants are non-empty ───────────────────────────────────────────

func TestEventsReasonConstants_NonEmpty(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"OptimizationApplied", events.ReasonOptimizationApplied},
		{"OptimizationBlocked", events.ReasonOptimizationBlocked},
		{"DryRunSimulated", events.ReasonDryRunSimulated},
		{"CircuitBreakerOpen", events.ReasonCircuitBreakerOpen},
		{"ScalingStarted", events.ReasonScalingStarted},
		{"ScalingCompleted", events.ReasonScalingCompleted},
		{"ScalingFailed", events.ReasonScalingFailed},
		{"AnomalyDetected", events.ReasonAnomalyDetected},
		{"PeakLoadPredicted", events.ReasonPeakLoadPredicted},
		{"GitOpsExportSucceeded", events.ReasonGitOpsExportSucceeded},
		{"GitOpsExportFailed", events.ReasonGitOpsExportFailed},
	}
	for _, tc := range cases {
		if tc.val == "" {
			t.Errorf("constant %s should not be empty", tc.name)
		}
	}
}

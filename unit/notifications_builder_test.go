package unit_test

import (
	"testing"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/notifications"
)

// ── ShouldNotifyEvent ─────────────────────────────────────────────────────────

func TestShouldNotifyEvent_DisabledNotifications(t *testing.T) {
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	if notifications.ShouldNotifyEvent(cfg, notifications.EventRecommendationApplied) {
		t.Error("disabled notifications should return false")
	}
}

func TestShouldNotifyEvent_NilNotifications(t *testing.T) {
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	cfg.Spec.Notifications = nil
	if notifications.ShouldNotifyEvent(cfg, notifications.EventOOMDetected) {
		t.Error("nil notifications should return false")
	}
}

func TestShouldNotifyEvent_EnabledNoFilter_NotifiesAll(t *testing.T) {
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Events:  nil,
	}
	if !notifications.ShouldNotifyEvent(cfg, notifications.EventRecommendationApplied) {
		t.Error("no event filter should notify all events")
	}
}

func TestShouldNotifyEvent_EnabledWithFilter_Matches(t *testing.T) {
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Events:  []optimizerv1alpha1.NotificationEventType{"OOMDetected"},
	}
	if !notifications.ShouldNotifyEvent(cfg, notifications.EventOOMDetected) {
		t.Error("OOMDetected should match filter")
	}
}

func TestShouldNotifyEvent_EnabledWithFilter_NoMatch(t *testing.T) {
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Events:  []optimizerv1alpha1.NotificationEventType{"OOMDetected"},
	}
	if notifications.ShouldNotifyEvent(cfg, notifications.EventSLAViolation) {
		t.Error("SLAViolation should not match OOMDetected filter")
	}
}

func TestShouldNotifyEvent_AllKnownEventTypes(t *testing.T) {
	cfg := &optimizerv1alpha1.OptimizerConfig{}
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{Enabled: true}

	evts := []notifications.EventType{
		notifications.EventRecommendationApplied,
		notifications.EventOOMDetected,
		notifications.EventMemoryLeakDetected,
		notifications.EventCircuitBreakerTriggered,
		notifications.EventSLAViolation,
		notifications.EventRollbackExecuted,
	}
	for _, e := range evts {
		if !notifications.ShouldNotifyEvent(cfg, e) {
			t.Errorf("event %s should be notified when no filter set", e)
		}
	}
}

// ── NewManagerBuilder ─────────────────────────────────────────────────────────

func TestNewManagerBuilder_ReturnsNonNil(t *testing.T) {
	b := notifications.NewManagerBuilder(nil, nil)
	if b == nil {
		t.Error("NewManagerBuilder should return non-nil")
	}
}

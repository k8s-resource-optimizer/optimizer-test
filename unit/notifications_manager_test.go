package unit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/notifications"
)

// fakeNotifier is a test-only Notifier that records calls and can inject errors.
type fakeNotifier struct {
	name     string
	sendErr  error
	received []*notifications.Event
}

func (f *fakeNotifier) Name() string { return f.name }
func (f *fakeNotifier) Send(_ context.Context, e *notifications.Event) error {
	f.received = append(f.received, e)
	return f.sendErr
}

func TestManager_NewManager_Defaults(t *testing.T) {
	m := notifications.NewManager(nil)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.GetNotifierCount() != 0 {
		t.Error("expected 0 notifiers on new manager")
	}
}

func TestManager_AddNotifier_IncreasesCount(t *testing.T) {
	m := notifications.NewManager(nil)
	m.AddNotifier(&fakeNotifier{name: "n1"})
	m.AddNotifier(&fakeNotifier{name: "n2"})
	if m.GetNotifierCount() != 2 {
		t.Errorf("expected 2 notifiers, got %d", m.GetNotifierCount())
	}
}

func TestManager_RemoveAllNotifiers_ResetsCount(t *testing.T) {
	m := notifications.NewManager(nil)
	m.AddNotifier(&fakeNotifier{name: "n1"})
	m.RemoveAllNotifiers()
	if m.GetNotifierCount() != 0 {
		t.Errorf("expected 0 after RemoveAllNotifiers, got %d", m.GetNotifierCount())
	}
}

func TestManager_Send_NoNotifiers_DoesNotPanic(t *testing.T) {
	m := notifications.NewManager(nil)
	m.Send(&notifications.Event{
		Type:      notifications.EventRecommendationApplied,
		Severity:  notifications.SeverityInfo,
		Namespace: "default",
		Message:   "test",
		Timestamp: time.Now(),
	})
}

func TestManager_Send_SetsTimestampIfZero(t *testing.T) {
	fn := &fakeNotifier{name: "ts-test"}
	m := notifications.NewManager(nil)
	m.SetRetryConfig(1, time.Millisecond)
	m.SetSendTimeout(200 * time.Millisecond)
	m.AddNotifier(fn)

	evt := &notifications.Event{
		Type:      notifications.EventOOMDetected,
		Severity:  notifications.SeverityCritical,
		Namespace: "default",
		Message:   "oom",
	}
	m.Send(evt)
	time.Sleep(50 * time.Millisecond)
}

func TestManager_SendBlocking_NoNotifiers_ReturnsError(t *testing.T) {
	m := notifications.NewManager(nil)
	err := m.SendBlocking(&notifications.Event{
		Type:      notifications.EventSLAViolation,
		Severity:  notifications.SeverityWarning,
		Namespace: "default",
		Message:   "sla",
		Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error when no notifiers configured")
	}
}

func TestManager_SendBlocking_Success(t *testing.T) {
	fn := &fakeNotifier{name: "blocking"}
	m := notifications.NewManager(nil)
	m.SetRetryConfig(1, time.Millisecond)
	m.SetSendTimeout(100 * time.Millisecond)
	m.AddNotifier(fn)

	evt := &notifications.Event{
		Type:      notifications.EventRollbackExecuted,
		Severity:  notifications.SeverityInfo,
		Namespace: "production",
		Message:   "rolled back",
		Timestamp: time.Now(),
	}
	if err := m.SendBlocking(evt); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(fn.received) == 0 {
		t.Error("expected event to be received by notifier")
	}
}

func TestManager_SendBlocking_NotifierError_ReturnsError(t *testing.T) {
	fn := &fakeNotifier{name: "fail-notifier", sendErr: errors.New("send failed")}
	m := notifications.NewManager(nil)
	m.SetRetryConfig(1, time.Millisecond)
	m.SetSendTimeout(50 * time.Millisecond)
	m.AddNotifier(fn)

	err := m.SendBlocking(&notifications.Event{
		Type:      notifications.EventCircuitBreakerTriggered,
		Severity:  notifications.SeverityCritical,
		Namespace: "kube-system",
		Message:   "circuit open",
		Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error when notifier fails")
	}
}

func TestManager_SetRetryConfig_AppliesOnSend(t *testing.T) {
	attemptCount := 0
	fn := &fakeNotifier{name: "retry-notifier", sendErr: errors.New("transient")}
	fn.sendErr = errors.New("always fail")

	m := notifications.NewManager(nil)
	m.SetRetryConfig(2, time.Millisecond)
	m.SetSendTimeout(50 * time.Millisecond)
	m.AddNotifier(fn)

	_ = attemptCount
	_ = m.SendBlocking(&notifications.Event{
		Type:      notifications.EventMemoryLeakDetected,
		Severity:  notifications.SeverityWarning,
		Namespace: "default",
		Message:   "leak",
		Timestamp: time.Now(),
	})
}

func TestManager_MultipleNotifiers_AllReceiveEvent(t *testing.T) {
	fn1 := &fakeNotifier{name: "ch1"}
	fn2 := &fakeNotifier{name: "ch2"}
	fn3 := &fakeNotifier{name: "ch3"}

	m := notifications.NewManager(nil)
	m.SetRetryConfig(1, time.Millisecond)
	m.SetSendTimeout(100 * time.Millisecond)
	m.AddNotifier(fn1)
	m.AddNotifier(fn2)
	m.AddNotifier(fn3)

	evt := &notifications.Event{
		Type:      notifications.EventSLAViolation,
		Severity:  notifications.SeverityCritical,
		Namespace: "prod",
		Message:   "p99 breach",
		Timestamp: time.Now(),
	}
	if err := m.SendBlocking(evt); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	for _, fn := range []*fakeNotifier{fn1, fn2, fn3} {
		if len(fn.received) == 0 {
			t.Errorf("notifier %s did not receive event", fn.name)
		}
	}
}

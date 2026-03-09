// Package unit_test contains black-box unit tests for the optimizer packages.
// Each test file targets one package and exercises its public API only.
// Tests are written as a separate module (optimizer-test) so they can be run
// independently from the main source tree with `go test ./unit/...`.
package unit_test

import (
	"errors"
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/safety"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newTestConfig creates a minimal OptimizerConfig with a CircuitBreakerConfig
// so we can control the threshold values in each test independently.
func newTestConfig(errorThreshold, successThreshold int, timeout string) *v1alpha1.OptimizerConfig {
	return &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			CircuitBreaker: &v1alpha1.CircuitBreakerConfig{
				Enabled:          true,
				ErrorThreshold:   errorThreshold,
				SuccessThreshold: successThreshold,
				Timeout:          timeout,
			},
		},
		Status: v1alpha1.OptimizerConfigStatus{
			CircuitState: v1alpha1.CircuitStateClosed,
		},
	}
}

func TestCircuitBreaker_InitiallyAllows(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(3, 2, "5m")

	if !cb.ShouldAllow(cfg) {
		t.Error("fresh circuit breaker should allow requests")
	}
}

func TestCircuitBreaker_OpensAfterThresholdFailures(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(3, 2, "5m")
	err := errors.New("simulated failure")

	// First two failures should not open
	cb.RecordFailure(cfg, err)
	if cfg.Status.CircuitState == v1alpha1.CircuitStateOpen {
		t.Error("circuit should still be closed after 1 failure")
	}
	cb.RecordFailure(cfg, err)
	if cfg.Status.CircuitState == v1alpha1.CircuitStateOpen {
		t.Error("circuit should still be closed after 2 failures")
	}

	// Third failure must open the circuit
	cb.RecordFailure(cfg, err)
	if cfg.Status.CircuitState != v1alpha1.CircuitStateOpen {
		t.Errorf("circuit must be Open after 3 consecutive failures, got %s", cfg.Status.CircuitState)
	}
}

func TestCircuitBreaker_BlocksWhenOpen(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(3, 2, "5m")
	err := errors.New("simulated failure")

	for i := 0; i < 3; i++ {
		cb.RecordFailure(cfg, err)
	}

	if cb.ShouldAllow(cfg) {
		t.Error("open circuit breaker must block requests")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(3, 2, "10ms")
	err := errors.New("simulated failure")

	for i := 0; i < 3; i++ {
		cb.RecordFailure(cfg, err)
	}

	// Simulate timeout elapsed
	past := metav1.NewTime(time.Now().Add(-100 * time.Millisecond))
	cfg.Status.LastUpdateTime = &past

	if !cb.ShouldAllow(cfg) {
		t.Error("circuit should allow one request in half-open state after timeout")
	}
	if cfg.Status.CircuitState != v1alpha1.CircuitStateHalfOpen {
		t.Errorf("expected HalfOpen state, got %s", cfg.Status.CircuitState)
	}
}

func TestCircuitBreaker_ClosesAfterSuccessThreshold(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(3, 2, "10ms")
	err := errors.New("simulated failure")

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure(cfg, err)
	}

	// Transition to half-open
	past := metav1.NewTime(time.Now().Add(-100 * time.Millisecond))
	cfg.Status.LastUpdateTime = &past
	cb.ShouldAllow(cfg) // triggers transition to HalfOpen

	// Record successes to close
	cb.RecordSuccess(cfg)
	if cfg.Status.CircuitState == v1alpha1.CircuitStateClosed {
		t.Error("should not close after only 1 success (threshold=2)")
	}
	cb.RecordSuccess(cfg)
	if cfg.Status.CircuitState != v1alpha1.CircuitStateClosed {
		t.Errorf("circuit should be Closed after %d successes, got %s", 2, cfg.Status.CircuitState)
	}
}

func TestCircuitBreaker_SuccessResetsErrorCount(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(3, 2, "5m")
	err := errors.New("simulated failure")

	cb.RecordFailure(cfg, err)
	cb.RecordFailure(cfg, err)
	cb.RecordSuccess(cfg)

	if cfg.Status.ConsecutiveErrors != 0 {
		t.Errorf("success should reset consecutive error count, got %d", cfg.Status.ConsecutiveErrors)
	}
}

func TestCircuitBreaker_FailureResetsSuccessCount(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(5, 3, "5m")
	err := errors.New("simulated failure")

	cb.RecordSuccess(cfg)
	cb.RecordSuccess(cfg)
	cb.RecordFailure(cfg, err)

	if cfg.Status.ConsecutiveSuccesses != 0 {
		t.Errorf("failure should reset consecutive success count, got %d", cfg.Status.ConsecutiveSuccesses)
	}
}

func TestCircuitBreaker_DisabledAlwaysAllows(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			CircuitBreaker: &v1alpha1.CircuitBreakerConfig{
				Enabled: false,
			},
		},
	}

	err := errors.New("simulated failure")
	for i := 0; i < 10; i++ {
		cb.RecordFailure(cfg, err)
	}

	if !cb.ShouldAllow(cfg) {
		t.Error("disabled circuit breaker must always allow requests")
	}
}

func TestCircuitBreaker_NilConfigAlwaysAllows(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			CircuitBreaker: nil,
		},
	}

	if !cb.ShouldAllow(cfg) {
		t.Error("nil circuit breaker config must always allow requests")
	}
}

func TestCircuitBreaker_CountersTracked(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cfg := newTestConfig(5, 3, "5m")
	err := errors.New("failure")

	cb.RecordSuccess(cfg)
	cb.RecordSuccess(cfg)
	cb.RecordFailure(cfg, err)

	if cfg.Status.TotalUpdatesApplied != 2 {
		t.Errorf("expected 2 total successes, got %d", cfg.Status.TotalUpdatesApplied)
	}
	if cfg.Status.TotalUpdatesFailed != 1 {
		t.Errorf("expected 1 total failure, got %d", cfg.Status.TotalUpdatesFailed)
	}
}

func TestCircuitBreaker_GetStateName(t *testing.T) {
	cb := safety.NewCircuitBreaker()
	cases := []struct {
		state    v1alpha1.CircuitState
		expected string
	}{
		{v1alpha1.CircuitStateClosed, "Closed"},
		{v1alpha1.CircuitStateOpen, "Open"},
		{v1alpha1.CircuitStateHalfOpen, "HalfOpen"},
	}

	for _, tc := range cases {
		got := cb.GetStateName(tc.state)
		if got != tc.expected {
			t.Errorf("GetStateName(%s) = %s, want %s", tc.state, got, tc.expected)
		}
	}
}

package unit_test

// Unit tests for MLForecasterConfig deepcopy and ScalerConfig default values.

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/forecaster"
)

// ── MLForecasterConfig.DeepCopy ────────────────────────────────────────────────

// Test 3a: DeepCopy on nil receiver returns nil safely
func TestMLForecasterConfig_DeepCopy_Nil(t *testing.T) {
	var cfg *v1alpha1.MLForecasterConfig
	got := cfg.DeepCopy()
	if got != nil {
		t.Errorf("expected nil DeepCopy of nil, got %+v", got)
	}
}

// Test 3b: DeepCopy copies all scalar fields correctly
func TestMLForecasterConfig_DeepCopy_AllFields(t *testing.T) {
	orig := &v1alpha1.MLForecasterConfig{
		Enabled:            true,
		ServiceURL:         "http://ml-service:8080",
		TimeoutSeconds:     30,
		ScaleUpThreshold:   0.80,
		ScaleDownThreshold: 0.20,
		CPUPerReplica:      0.30,
		MinReplicas:        2,
		MaxReplicas:        8,
	}

	copy := orig.DeepCopy()

	if copy == orig {
		t.Fatal("DeepCopy returned the same pointer (not a copy)")
	}
	if copy.Enabled != orig.Enabled {
		t.Errorf("Enabled: want %v, got %v", orig.Enabled, copy.Enabled)
	}
	if copy.ServiceURL != orig.ServiceURL {
		t.Errorf("ServiceURL: want %q, got %q", orig.ServiceURL, copy.ServiceURL)
	}
	if copy.TimeoutSeconds != orig.TimeoutSeconds {
		t.Errorf("TimeoutSeconds: want %d, got %d", orig.TimeoutSeconds, copy.TimeoutSeconds)
	}
	if copy.ScaleUpThreshold != orig.ScaleUpThreshold {
		t.Errorf("ScaleUpThreshold: want %f, got %f", orig.ScaleUpThreshold, copy.ScaleUpThreshold)
	}
	if copy.ScaleDownThreshold != orig.ScaleDownThreshold {
		t.Errorf("ScaleDownThreshold: want %f, got %f", orig.ScaleDownThreshold, copy.ScaleDownThreshold)
	}
	if copy.CPUPerReplica != orig.CPUPerReplica {
		t.Errorf("CPUPerReplica: want %f, got %f", orig.CPUPerReplica, copy.CPUPerReplica)
	}
	if copy.MinReplicas != orig.MinReplicas {
		t.Errorf("MinReplicas: want %d, got %d", orig.MinReplicas, copy.MinReplicas)
	}
	if copy.MaxReplicas != orig.MaxReplicas {
		t.Errorf("MaxReplicas: want %d, got %d", orig.MaxReplicas, copy.MaxReplicas)
	}
}

// Test 3c: mutating the copy does not affect the original
func TestMLForecasterConfig_DeepCopy_Independence(t *testing.T) {
	orig := &v1alpha1.MLForecasterConfig{
		Enabled:     true,
		ServiceURL:  "http://original:8080",
		MaxReplicas: 5,
	}
	copy := orig.DeepCopy()

	copy.ServiceURL  = "http://modified:9090"
	copy.MaxReplicas = 99
	copy.Enabled     = false

	if orig.ServiceURL != "http://original:8080" {
		t.Errorf("original ServiceURL was mutated: %s", orig.ServiceURL)
	}
	if orig.MaxReplicas != 5 {
		t.Errorf("original MaxReplicas was mutated: %d", orig.MaxReplicas)
	}
	if !orig.Enabled {
		t.Error("original Enabled was mutated")
	}
}

// Test 3d: DeepCopyInto with zero-value source does not panic
func TestMLForecasterConfig_DeepCopyInto_ZeroValue(t *testing.T) {
	in  := &v1alpha1.MLForecasterConfig{}
	out := &v1alpha1.MLForecasterConfig{}
	in.DeepCopyInto(out)
	if out.Enabled != false || out.ServiceURL != "" {
		t.Errorf("unexpected non-zero value after copying zero struct: %+v", out)
	}
}

// ── ScalerConfig.withDefaults (via forecaster package) ────────────────────────
// withDefaults is tested indirectly through Decide() — zero-value ScalerConfig
// must still produce sensible scaling decisions using the documented defaults.

// Test 3e: zero-value ScalerConfig uses documented defaults (0.75 up, 0.30 down, 0.25 per-replica)
func TestScalerConfig_Defaults_ScaleUpApplied(t *testing.T) {
	// p90=0.80 should trigger scale-up with default ScaleUpThreshold=0.75
	forecast := makeFlatForecast(15, 0.70, 0.80)
	d := forecaster.Decide(forecast, 2, forecaster.ScalerConfig{}) // all zeros → defaults

	if !d.ScaleUp {
		t.Error("expected ScaleUp=true with p90=0.80 and default threshold 0.75")
	}
}

// Test 3f: zero-value ScalerConfig uses default ScaleDownThreshold=0.30
func TestScalerConfig_Defaults_ScaleDownApplied(t *testing.T) {
	// p50=0.10 should trigger scale-down with default ScaleDownThreshold=0.30
	forecast := makeFlatForecast(15, 0.10, 0.15)
	d := forecaster.Decide(forecast, 5, forecaster.ScalerConfig{})

	if !d.ScaleDown {
		t.Error("expected ScaleDown=true with p50=0.10 and default threshold 0.30")
	}
}

// Test 3g: zero-value ScalerConfig respects default MinReplicas=1
func TestScalerConfig_Defaults_MinReplicas(t *testing.T) {
	forecast := makeFlatForecast(15, 0.01, 0.02)
	d := forecaster.Decide(forecast, 5, forecaster.ScalerConfig{})

	if d.DesiredReplicas < 1 {
		t.Errorf("expected DesiredReplicas >= 1 (default min), got %d", d.DesiredReplicas)
	}
}

// Test 3h: zero-value ScalerConfig respects default MaxReplicas=10
func TestScalerConfig_Defaults_MaxReplicas(t *testing.T) {
	forecast := makeFlatForecast(15, 0.99, 0.99)
	d := forecaster.Decide(forecast, 1, forecaster.ScalerConfig{})

	if d.DesiredReplicas > 10 {
		t.Errorf("expected DesiredReplicas <= 10 (default max), got %d", d.DesiredReplicas)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────────

func makeFlatForecast(n int, median, high float64) []forecaster.ForecastPoint {
	pts := make([]forecaster.ForecastPoint, n)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: median * 0.8, Median: median, High: high}
	}
	return pts
}

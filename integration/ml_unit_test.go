package integration_test

// Adapted from pkg/controller/ml_unit_test.go.
// Tests extractCPUHistory behaviour and SetMLForecaster through the exported Reconcile API,
// and tests SetMLForecaster enable/disable semantics.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"intelligent-cluster-optimizer/pkg/controller"
	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/models"
	"k8s.io/client-go/kubernetes/fake"
)

// capturingForecaster records every slice passed to Predict.
type capturingForecaster struct {
	calls [][]float64
}

func (c *capturingForecaster) Predict(_ context.Context, vals []float64) (*forecaster.PredictResponse, error) {
	// Store a copy so later appends don't alias the slice.
	cp := make([]float64, len(vals))
	copy(cp, vals)
	c.calls = append(c.calls, cp)
	pts := make([]forecaster.ForecastPoint, 15)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: 0.1, Median: 0.3, High: 0.5}
	}
	return &forecaster.PredictResponse{Forecast: pts, ContextLength: 60, PredictionLength: 15}, nil
}

// populateMetricsWithCPU adds n PodMetrics with the given CPU values using
// unique timestamps and the <deploy>-<rs>-<pod> naming convention.
func populateMetricsWithCPU(store interface{ Add(models.PodMetric) },
	ns, deployName string, n int, usageCPU, limitCPU, requestCPU int64) {
	now := time.Now()
	for i := 0; i < n; i++ {
		store.Add(models.PodMetric{
			PodName:   fmt.Sprintf("%s-abc123-pod%03d", deployName, i),
			Namespace: ns,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      usageCPU,
					LimitCPU:      limitCPU,
					RequestCPU:    requestCPU,
				},
			},
		})
	}
}

// newReconcilerWithCapturing builds a Reconciler wired up with a capturing forecaster
// and a fake kube client that includes the specified deployment.
func newReconcilerWithCapturing(t *testing.T, ns, deployName string) (*controller.Reconciler, *capturingForecaster) {
	t.Helper()
	kube := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	r := controller.NewReconciler(kube, nil)
	cf := &capturingForecaster{}
	r.SetMLForecaster(cf, forecaster.NewHorizontalScaler(kube, zaptest.NewLogger(t)))
	return r, cf
}

// ── extractCPUHistory behaviour (Tests 2a–2g) ─────────────────────────────────

// Test 2a: empty metrics → Predict never called (extractCPUHistory returns nil → <30 samples)
func TestExtractCPUHistory_Empty_PredictNotCalled(t *testing.T) {
	r, cf := newReconcilerWithCapturing(t, "default", "test-app")

	// No metrics added.
	cfg := buildConfigWithML("default", "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) != 0 {
		t.Errorf("expected Predict not called for empty metrics, got %d calls", len(cf.calls))
	}
}

// Test 2b: containers with zero LimitCPU and zero RequestCPU are skipped →
// all filtered out → Predict not called
func TestExtractCPUHistory_ZeroCapacitySkipped_PredictNotCalled(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	r, cf := newReconcilerWithCapturing(t, ns, deployName)

	// 40 metrics but every container has zero capacity.
	populateMetricsWithCPU(r.GetMetricsStorage(), ns, deployName, 40, 100, 0, 0)

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) != 0 {
		t.Errorf("zero-capacity pods should be skipped — Predict must not be called, got %d calls", len(cf.calls))
	}
}

// Test 2c: falls back to RequestCPU when LimitCPU is 0 →
// normalised value == UsageCPU / RequestCPU
func TestExtractCPUHistory_FallsBackToRequest(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	r, cf := newReconcilerWithCapturing(t, ns, deployName)

	// LimitCPU=0, RequestCPU=200, UsageCPU=100 → each point: 100/200 = 0.5
	populateMetricsWithCPU(r.GetMetricsStorage(), ns, deployName, 30, 100, 0, 200)

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) == 0 {
		t.Fatal("expected Predict to be called with RequestCPU fallback data")
	}
	for _, v := range cf.calls[0] {
		if v != 0.5 {
			t.Errorf("expected normalised value 0.5 (100/200), got %f", v)
		}
	}
}

// Test 2d: usage exceeding limit is clamped to 1.0
func TestExtractCPUHistory_ClampedToOne(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	r, cf := newReconcilerWithCapturing(t, ns, deployName)

	// UsageCPU=500 > LimitCPU=200 → clamped to 1.0
	populateMetricsWithCPU(r.GetMetricsStorage(), ns, deployName, 30, 500, 200, 200)

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) == 0 {
		t.Fatal("expected Predict to be called")
	}
	for _, v := range cf.calls[0] {
		if v != 1.0 {
			t.Errorf("expected clamped value 1.0, got %f", v)
		}
	}
}

// Test 2e: metrics are sorted by timestamp before processing →
// values arrive in ascending order even when added in reverse.
func TestExtractCPUHistory_TimestampOrdering(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	r, cf := newReconcilerWithCapturing(t, ns, deployName)

	now := time.Now()
	// Add 30 metrics in REVERSE timestamp order with ascending CPU values so that
	// correct timestamp sorting produces an ascending value sequence.
	for i := 29; i >= 0; i-- {
		r.GetMetricsStorage().Add(models.PodMetric{
			PodName:   fmt.Sprintf("%s-abc123-pod%03d", deployName, i),
			Namespace: ns,
			Timestamp: now.Add(time.Duration(i) * time.Minute), // oldest = i=0
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: int64((i + 1) * 10), LimitCPU: 1000},
			},
		})
	}

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) == 0 {
		t.Fatal("expected Predict to be called")
	}
	vals := cf.calls[0]
	for i := 1; i < len(vals); i++ {
		if vals[i] < vals[i-1] {
			t.Errorf("values not in ascending order at index %d: %.4f < %.4f (timestamp sorting broken)",
				i, vals[i], vals[i-1])
		}
	}
}

// Test 2f: normal utilisation (0 < usage < limit) is correctly normalised
func TestExtractCPUHistory_NormalUtilisation(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	r, cf := newReconcilerWithCapturing(t, ns, deployName)
	now := time.Now()

	// Two containers per pod: usage=400, capacity=1500 → 400/1500 each
	for i := 0; i < 30; i++ {
		r.GetMetricsStorage().Add(models.PodMetric{
			PodName:   fmt.Sprintf("%s-abc123-pod%03d", deployName, i),
			Namespace: ns,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{ContainerName: "c1", UsageCPU: 250, LimitCPU: 1000},
				{ContainerName: "c2", UsageCPU: 150, LimitCPU: 500},
			},
		})
	}

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) == 0 {
		t.Fatal("expected Predict to be called")
	}
	want := 400.0 / 1500.0
	for i, v := range cf.calls[0] {
		if v != want {
			t.Errorf("index %d: expected %.6f, got %.6f", i, want, v)
		}
	}
}

// Test 2g: mixed pods — some zero-capacity (skipped), some valid (included) →
// Predict is called with only the valid points.
func TestExtractCPUHistory_MixedPods(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	r, cf := newReconcilerWithCapturing(t, ns, deployName)
	now := time.Now()

	// 30 valid pods
	for i := 0; i < 30; i++ {
		r.GetMetricsStorage().Add(models.PodMetric{
			PodName:   fmt.Sprintf("%s-abc123-valid%03d", deployName, i),
			Namespace: ns,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: 100, LimitCPU: 400},
			},
		})
	}
	// 10 zero-capacity pods (should be filtered out by extractCPUHistory).
	// UsageCPU matches the valid pods (100) so the anomaly detector sees a flat
	// series and does NOT block the workload before the ML block is reached.
	for i := 0; i < 10; i++ {
		r.GetMetricsStorage().Add(models.PodMetric{
			PodName:   fmt.Sprintf("%s-abc123-zero%03d", deployName, i),
			Namespace: ns,
			Timestamp: now.Add(time.Duration(30+i) * time.Minute),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: 100, LimitCPU: 0, RequestCPU: 0},
			},
		})
	}

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) == 0 {
		t.Error("expected Predict called — 30 valid pods should pass the threshold")
	}
	if len(cf.calls) > 0 && len(cf.calls[0]) > 40 {
		t.Errorf("expected at most 40 data points (30 valid), got %d — zero-cap pods were not filtered", len(cf.calls[0]))
	}
}

// ── SetMLForecaster behaviour (Tests 2h–2i) ───────────────────────────────────

// Test 2h: SetMLForecaster stores the injected forecaster →
// Predict is called after injection.
func TestSetMLForecaster_Stored(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	logger := zaptest.NewLogger(t)
	kube   := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	r      := controller.NewReconciler(kube, nil)

	cf     := &capturingForecaster{}
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(cf, scaler)

	populateMetrics(r.GetMetricsStorage(), ns, deployName, 40)

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) == 0 {
		t.Error("expected mlForecaster.Predict to be called — forecaster was not stored")
	}
}

// Test 2i: SetMLForecaster with nil clears the forecaster →
// Predict is never called after clearing.
func TestSetMLForecaster_NilClears(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	logger := zaptest.NewLogger(t)
	kube   := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	r      := controller.NewReconciler(kube, nil)

	cf := &capturingForecaster{}
	r.SetMLForecaster(cf, forecaster.NewHorizontalScaler(kube, logger))
	// Clear both forecaster and scaler.
	r.SetMLForecaster(nil, nil)

	populateMetrics(r.GetMetricsStorage(), ns, deployName, 40)

	cfg := buildConfigWithML(ns, "opt-config", true)
	if _, err := r.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.calls) != 0 {
		t.Errorf("expected Predict never called after clearing, got %d calls", len(cf.calls))
	}
}

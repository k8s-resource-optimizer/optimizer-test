package integration_test

// Integration tests 4-6.
//
// Test 4: FallbackForecaster with a real HTTP server returning 500 — verifies the
//         full HTTP→error→Holt-Winters chain under realistic conditions.
// Test 5: Reconcile loop with a mock CpuForecaster injected — verifies orchestration:
//         metrics populated → Predict called, dry-run → Scale NOT called.
// Test 6: MLForecaster.Enabled=false — forecaster never called even when injected.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/controller"
	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

// ── Test 4: FallbackForecaster with unavailable ML service ─────────────────────

// Test 4a: real HTTP server returning 500 → FallbackForecaster uses Holt-Winters, no error
func TestIntegration_Fallback_MLServiceUnavailable(t *testing.T) {
	// Spin up a real HTTP server that always returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"service unavailable"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	logger    := zaptest.NewLogger(t)
	mlClient  := forecaster.NewForecastClient(srv.URL, 2*time.Second, logger)
	hwFcaster := forecaster.NewHoltWintersForecaster(logger)
	fb        := forecaster.NewFallbackForecaster(mlClient, hwFcaster, logger)

	samples := make([]float64, 60)
	for i := range samples {
		samples[i] = 0.1 + 0.3*float64(i)/60.0
	}

	resp, err := fb.Predict(context.Background(), samples)
	if err != nil {
		t.Fatalf("FallbackForecaster must not return an error when ML is down: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response from Holt-Winters fallback")
	}
	if len(resp.Forecast) == 0 {
		t.Error("expected at least one forecast point from Holt-Winters")
	}
}

// Test 4b: real HTTP server with timeout → FallbackForecaster still succeeds via Holt-Winters
func TestIntegration_Fallback_MLServiceTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // much longer than the client timeout
	}))
	defer slow.Close()

	logger    := zaptest.NewLogger(t)
	mlClient  := forecaster.NewForecastClient(slow.URL, 50*time.Millisecond, logger) // very short timeout
	hwFcaster := forecaster.NewHoltWintersForecaster(logger)
	fb        := forecaster.NewFallbackForecaster(mlClient, hwFcaster, logger)

	samples := make([]float64, 60)
	for i := range samples {
		samples[i] = 0.2
	}

	resp, err := fb.Predict(context.Background(), samples)
	if err != nil {
		t.Fatalf("expected Holt-Winters fallback on timeout, got error: %v", err)
	}
	if len(resp.Forecast) == 0 {
		t.Error("expected forecast points from fallback")
	}
}

// Test 4c: real HTTP server returning valid JSON → FallbackForecaster returns ML result
func TestIntegration_Fallback_MLServiceHealthy(t *testing.T) {
	pts := make([]forecaster.ForecastPoint, 15)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: 0.1, Median: 0.3, High: 0.5}
	}
	mlResp := forecaster.PredictResponse{
		Forecast: pts, ContextLength: 60, PredictionLength: 15, InferenceMs: 12.5,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mlResp)
	}))
	defer srv.Close()

	logger    := zaptest.NewLogger(t)
	mlClient  := forecaster.NewForecastClient(srv.URL, 2*time.Second, logger)
	hwFcaster := forecaster.NewHoltWintersForecaster(logger)
	fb        := forecaster.NewFallbackForecaster(mlClient, hwFcaster, logger)

	samples := make([]float64, 60)
	resp, err := fb.Predict(context.Background(), samples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Forecast) != 15 {
		t.Errorf("expected 15 forecast points from ML service, got %d", len(resp.Forecast))
	}
	if resp.InferenceMs != 12.5 {
		t.Errorf("expected ML response (InferenceMs=12.5), got %f — may have fallen back", resp.InferenceMs)
	}
}

// ── Test 5: Reconcile loop with mock CpuForecaster ─────────────────────────────

// countingForecaster records how many times Predict was called.
type countingForecaster struct {
	called   int
	response *forecaster.PredictResponse
}

func (c *countingForecaster) Predict(_ context.Context, _ []float64) (*forecaster.PredictResponse, error) {
	c.called++
	pts := make([]forecaster.ForecastPoint, 15)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: 0.1, Median: 0.3, High: 0.5}
	}
	return &forecaster.PredictResponse{Forecast: pts, ContextLength: 60, PredictionLength: 15}, nil
}

// buildConfigWithML returns a minimal OptimizerConfig with MLForecaster enabled.
// Status.Phase must be non-empty to pass the "Initializing" early-return guard in Reconcile.
func buildConfigWithML(ns, name string, dryRun bool) *optimizerv1alpha1.OptimizerConfig {
	return &optimizerv1alpha1.OptimizerConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{ns},
			DryRun:           dryRun,
			MLForecaster: &optimizerv1alpha1.MLForecasterConfig{
				Enabled:            true,
				ScaleUpThreshold:   0.75,
				ScaleDownThreshold: 0.30,
				CPUPerReplica:      0.25,
				MinReplicas:        1,
				MaxReplicas:        10,
			},
		},
		Status: optimizerv1alpha1.OptimizerConfigStatus{
			Phase: optimizerv1alpha1.OptimizerPhaseActive,
		},
	}
}

// populateMetrics adds n CPU metric data points for the given deployment to the store.
// Pod name follows the Kubernetes pattern "<deploy>-<rs-hash>-<pod-hash>" so that
// extractWorkloadName() correctly strips the last two segments and returns deployName.
func populateMetrics(store *storage.InMemoryStorage, ns, deployName string, n int) {
	now := time.Now()
	// Two fake pods — realistic spread across replica set
	podNames := []string{
		deployName + "-abc123-pod01",
		deployName + "-abc123-pod02",
	}
	for i := 0; i < n; i++ {
		podName := podNames[i%len(podNames)]
		store.Add(models.PodMetric{
			PodName:   podName,
			Namespace: ns,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      int64(100 + i*5), // gently rising CPU
					LimitCPU:      1000,
					RequestCPU:    500,
					UsageMemory:   128 * 1024 * 1024,
					LimitMemory:   256 * 1024 * 1024,
				},
			},
		})
	}
}

// newDeploymentForTest creates a fake Deployment in the given namespace.
func newDeploymentForTest(ns, name string, replicas int32) *appsv1.Deployment {
	r := replicas
	cpu := resource.MustParse("100m")
	mem := resource.MustParse("128Mi")
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &r,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "nginx:latest",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem},
						},
					}},
				},
			},
		},
	}
}

// Test 5a: reconcile with ≥30 metrics → mock Predict() is called
func TestIntegration_Reconcile_MLForecasterCalled(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)

	kube   := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	logger := zaptest.NewLogger(t)
	r      := controller.NewReconciler(kube, nil)

	mock   := &countingForecaster{}
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(mock, scaler)

	// Populate storage with 40 data points (>30 minimum)
	populateMetrics(r.GetMetricsStorage(), ns, deployName, 40)

	// DryRun=true prevents the vertical scaler from hanging on the fake kube client
	cfg := buildConfigWithML(ns, "opt-config", true)
	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if mock.called == 0 {
		t.Error("expected mlForecaster.Predict to be called at least once, was never called")
	}
}

// Test 5b: fewer than 30 metrics → Predict() is NOT called (not enough history)
func TestIntegration_Reconcile_InsufficientMetrics_PredictNotCalled(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)

	kube   := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	logger := zaptest.NewLogger(t)
	r      := controller.NewReconciler(kube, nil)

	mock   := &countingForecaster{}
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(mock, scaler)

	// Only 10 data points — below the 30 minimum
	populateMetrics(r.GetMetricsStorage(), ns, deployName, 10)

	// DryRun=true prevents vertical scaler from hanging on the fake kube client
	cfg := buildConfigWithML(ns, "opt-config", true)
	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if mock.called != 0 {
		t.Errorf("expected Predict NOT to be called with <30 samples, got called %d times", mock.called)
	}
}

// Test 5c: dry-run mode → Predict() is called but actual Scale() is not applied
func TestIntegration_Reconcile_DryRun_ScaleSkipped(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)
	initialReplicas := int32(2)

	kube := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, initialReplicas))
	logger := zaptest.NewLogger(t)
	r := controller.NewReconciler(kube, nil)

	// Use a high-CPU forecaster that would normally trigger scale-up
	highCPU := &highCPUForecaster{}
	scaler  := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(highCPU, scaler)

	populateMetrics(r.GetMetricsStorage(), ns, deployName, 40)

	// dry-run = true
	cfg := buildConfigWithML(ns, "opt-config", true)
	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	// Verify replica count did NOT change
	dep, err := kube.AppsV1().Deployments(ns).Get(context.Background(), deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}
	if *dep.Spec.Replicas != initialReplicas {
		t.Errorf("dry-run: expected replicas=%d (no change), got %d",
			initialReplicas, *dep.Spec.Replicas)
	}
}

// highCPUForecaster always returns a p90=0.99 forecast (guaranteed scale-up trigger).
type highCPUForecaster struct{ called int }

func (h *highCPUForecaster) Predict(_ context.Context, _ []float64) (*forecaster.PredictResponse, error) {
	h.called++
	pts := make([]forecaster.ForecastPoint, 15)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: 0.8, Median: 0.9, High: 0.99}
	}
	return &forecaster.PredictResponse{Forecast: pts, ContextLength: 60, PredictionLength: 15}, nil
}

// ── Test 6: MLForecaster disabled flag ────────────────────────────────────────

// Test 6a: MLForecaster.Enabled=false → Predict never called even when injected
func TestIntegration_MLForecasterDisabled_PredictNotCalled(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)

	kube   := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	logger := zaptest.NewLogger(t)
	r      := controller.NewReconciler(kube, nil)

	mock   := &countingForecaster{}
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(mock, scaler)

	populateMetrics(r.GetMetricsStorage(), ns, deployName, 40)

	// Build config with MLForecaster.Enabled=false; DryRun=true prevents vertical scaler hang
	cfg := buildConfigWithML(ns, "opt-config", true)
	cfg.Spec.MLForecaster.Enabled = false

	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if mock.called != 0 {
		t.Errorf("expected Predict never called when Enabled=false, got called %d times", mock.called)
	}
}

// Test 6b: MLForecaster config nil → Predict never called
func TestIntegration_MLForecasterConfigNil_PredictNotCalled(t *testing.T) {
	const (
		ns         = "default"
		deployName = "test-app"
	)

	kube   := fake.NewSimpleClientset(newDeploymentForTest(ns, deployName, 2))
	logger := zaptest.NewLogger(t)
	r      := controller.NewReconciler(kube, nil)

	mock   := &countingForecaster{}
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(mock, scaler)

	populateMetrics(r.GetMetricsStorage(), ns, deployName, 40)

	// DryRun=true prevents vertical scaler hang; MLForecaster=nil is the key condition
	cfg := buildConfigWithML(ns, "opt-config", true)
	cfg.Spec.MLForecaster = nil // no MLForecaster config at all

	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if mock.called != 0 {
		t.Errorf("expected Predict never called when MLForecaster=nil, got called %d times", mock.called)
	}
}

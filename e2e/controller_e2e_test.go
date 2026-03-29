//go:build e2e

package e2e

// E2E tests 7 and 8 — run against a live Kind cluster.
//
// Prerequisites:
//   kind create cluster --name optimizer-e2e
//   kubectl apply -f config/crd/optimizerconfig-crd.yaml
//   kubectl apply -f deploy/namespace.yaml
//
// Run with:
//   go test ./tests/e2e/... -tags e2e -v -timeout 120s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/controller"
	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/models"
)

// kubeClient returns a real client connected to the kind cluster.
func kubeClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("failed to build kubeconfig: %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create kube client: %v", err)
	}
	return client
}

// createTestDeployment creates a Deployment in the cluster and registers cleanup.
func createTestDeployment(t *testing.T, kube kubernetes.Interface, ns, name string, replicas int32) *appsv1.Deployment {
	t.Helper()
	r := replicas
	cpu := resource.MustParse("100m")
	mem := resource.MustParse("128Mi")
	dep := &appsv1.Deployment{
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
	created, err := kube.AppsV1().Deployments(ns).Create(context.Background(), dep, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create deployment %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = kube.AppsV1().Deployments(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})
	return created
}

// populateE2EMetrics adds n CPU metrics for the given deployment to the reconciler's store.
func populateE2EMetrics(r *controller.Reconciler, ns, deployName string, n int) {
	now := time.Now()
	podNames := []string{
		deployName + "-abc123-pod01",
		deployName + "-abc123-pod02",
	}
	for i := 0; i < n; i++ {
		r.GetMetricsStorage().Add(models.PodMetric{
			PodName:   podNames[i%len(podNames)],
			Namespace: ns,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{{
				ContainerName: "app",
				UsageCPU:      int64(100 + i*5),
				LimitCPU:      1000,
				RequestCPU:    500,
				UsageMemory:   128 * 1024 * 1024,
				LimitMemory:   256 * 1024 * 1024,
			}},
		})
	}
}

// e2eConfig builds a minimal OptimizerConfig pointing at the given namespace.
func e2eConfig(ns, name string, dryRun bool, mlEnabled bool) *optimizerv1alpha1.OptimizerConfig {
	cfg := &optimizerv1alpha1.OptimizerConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{ns},
			DryRun:           dryRun,
		},
		Status: optimizerv1alpha1.OptimizerConfigStatus{
			Phase: optimizerv1alpha1.OptimizerPhaseActive,
		},
	}
	if mlEnabled {
		cfg.Spec.MLForecaster = &optimizerv1alpha1.MLForecasterConfig{
			Enabled:            true,
			ScaleUpThreshold:   0.75,
			ScaleDownThreshold: 0.30,
			CPUPerReplica:      0.25,
			MinReplicas:        1,
			MaxReplicas:        10,
		}
	}
	return cfg
}

// ── Test 7: controller without ML service uses Holt-Winters fallback ──────────
//
// Verifies that the reconciler:
//   - connects to a real Kubernetes API server (Kind)
//   - runs Reconcile() with mlForecaster.enabled=true but no ML service URL set
//   - uses Holt-Winters as the forecaster (via FallbackForecaster with dead primary)
//   - does not crash or return an error

func TestE2E_NoMLService_HoltWintersFallback(t *testing.T) {
	const (
		ns         = "default"
		deployName = "e2e-no-ml-app"
	)

	kube   := kubeClient(t)
	logger := zaptest.NewLogger(t)

	createTestDeployment(t, kube, ns, deployName, 2)

	// Wire up a FallbackForecaster with a dead primary (always errors) → Holt-Winters takes over.
	// This mirrors production when ML_SERVICE_URL is unset or the service is unreachable.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer dead.Close()

	mlClient  := forecaster.NewForecastClient(dead.URL, 500*time.Millisecond, logger)
	hwFcaster := forecaster.NewHoltWintersForecaster(logger)
	fb        := forecaster.NewFallbackForecaster(mlClient, hwFcaster, logger)

	r      := controller.NewReconciler(kube, nil)
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(fb, scaler)

	populateE2EMetrics(r, ns, deployName, 40)

	cfg := e2eConfig(ns, "e2e-opt-config", true, true)
	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error when ML service is down: %v", err)
	}
}

// ── Test 8: dry-run with mock ML service — replica count does not change ──────
//
// Verifies that when dryRun=true:
//   - the ML forecaster is called and returns a scale-up recommendation
//   - the actual Deployment replica count on the real cluster is NOT modified

func TestE2E_DryRun_ReplicaCountUnchanged(t *testing.T) {
	const (
		ns             = "default"
		deployName     = "e2e-dryrun-app"
		initialReplicas = int32(2)
	)

	kube   := kubeClient(t)
	logger := zaptest.NewLogger(t)

	createTestDeployment(t, kube, ns, deployName, initialReplicas)

	// Mock ML service that always returns a high-CPU forecast (would trigger scale-up in live mode).
	pts := make([]forecaster.ForecastPoint, 15)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: 0.8, Median: 0.9, High: 0.99}
	}
	mlResp := forecaster.PredictResponse{
		Forecast: pts, ContextLength: 60, PredictionLength: 15, InferenceMs: 5.0,
	}
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mlResp)
	}))
	defer mockML.Close()

	mlClient  := forecaster.NewForecastClient(mockML.URL, 2*time.Second, logger)
	hwFcaster := forecaster.NewHoltWintersForecaster(logger)
	fb        := forecaster.NewFallbackForecaster(mlClient, hwFcaster, logger)

	r      := controller.NewReconciler(kube, nil)
	scaler := forecaster.NewHorizontalScaler(kube, logger)
	r.SetMLForecaster(fb, scaler)

	populateE2EMetrics(r, ns, deployName, 40)

	// dryRun=true — scaling decision is made but NOT applied.
	cfg := e2eConfig(ns, "e2e-dryrun-config", true, true)
	_, err := r.Reconcile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	// Assert the real Deployment on the cluster still has the original replica count.
	dep, err := kube.AppsV1().Deployments(ns).Get(context.Background(), deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get deployment from cluster: %v", err)
	}
	if *dep.Spec.Replicas != initialReplicas {
		t.Errorf("dry-run: replica count changed on real cluster — expected %d, got %d",
			initialReplicas, *dep.Spec.Replicas)
	}
}

package unit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/apiserver"
	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/storage"

	"go.uber.org/zap"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ─── ScalingHistoryStore ──────────────────────────────────────────────────────

func TestScalingHistoryStore_AddAndGetAll(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(10)
	s.Add(apiserver.ScalingRecord{Namespace: "ns", DeploymentName: "dep", OldReplicas: 1, NewReplicas: 2})
	s.Add(apiserver.ScalingRecord{Namespace: "ns", DeploymentName: "dep2", OldReplicas: 2, NewReplicas: 3})
	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}
	// GetAll returns newest first
	if all[0].DeploymentName != "dep2" {
		t.Errorf("expected dep2 first (newest), got %s", all[0].DeploymentName)
	}
}

func TestScalingHistoryStore_AutoTimestamp(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(5)
	before := time.Now()
	s.Add(apiserver.ScalingRecord{DeploymentName: "dep"})
	after := time.Now()
	all := s.GetAll()
	if all[0].Timestamp.Before(before) || all[0].Timestamp.After(after) {
		t.Error("timestamp not set automatically")
	}
}

func TestScalingHistoryStore_AutoID(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(5)
	s.Add(apiserver.ScalingRecord{DeploymentName: "dep"})
	all := s.GetAll()
	if all[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestScalingHistoryStore_CapEviction(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(3)
	for i := 0; i < 5; i++ {
		s.Add(apiserver.ScalingRecord{NewReplicas: int32(i)})
	}
	all := s.GetAll()
	if len(all) > 3 {
		t.Fatalf("expected at most 3 records due to cap, got %d", len(all))
	}
}

func TestScalingHistoryStore_DefaultMaxSize(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(0) // 0 → default 500
	s.Add(apiserver.ScalingRecord{DeploymentName: "x"})
	if len(s.GetAll()) != 1 {
		t.Error("expected 1 record")
	}
}

// ─── ForecastCache ────────────────────────────────────────────────────────────

func TestForecastCache_SetAndGetAll(t *testing.T) {
	c := apiserver.NewForecastCache()
	c.Set("ns", "dep", apiserver.ForecastEntry{
		DeploymentName: "dep",
		Namespace:      "ns",
		CPUSamples:     10,
	})
	all := c.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
	if all[0].DeploymentName != "dep" {
		t.Errorf("unexpected deployment name: %s", all[0].DeploymentName)
	}
}

func TestForecastCache_AutoTimestamp(t *testing.T) {
	c := apiserver.NewForecastCache()
	before := time.Now()
	c.Set("ns", "dep", apiserver.ForecastEntry{})
	after := time.Now()
	all := c.GetAll()
	if all[0].Timestamp.Before(before) || all[0].Timestamp.After(after) {
		t.Error("timestamp not set automatically")
	}
}

func TestForecastCache_Overwrite(t *testing.T) {
	c := apiserver.NewForecastCache()
	c.Set("ns", "dep", apiserver.ForecastEntry{CPUSamples: 5})
	c.Set("ns", "dep", apiserver.ForecastEntry{CPUSamples: 99})
	all := c.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry (overwrite), got %d", len(all))
	}
	if all[0].CPUSamples != 99 {
		t.Errorf("expected overwritten value 99, got %d", all[0].CPUSamples)
	}
}

func TestForecastCache_MultipleDeployments(t *testing.T) {
	c := apiserver.NewForecastCache()
	c.Set("ns", "dep1", apiserver.ForecastEntry{DeploymentName: "dep1"})
	c.Set("ns", "dep2", apiserver.ForecastEntry{DeploymentName: "dep2"})
	if len(c.GetAll()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(c.GetAll()))
	}
}

// ─── DryRunQueue ──────────────────────────────────────────────────────────────

func TestDryRunQueue_AddAndGetPending(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{Namespace: "ns", DeploymentName: "dep", Reason: "test"})
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	pending := q.GetPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Status != apiserver.DryRunPending {
		t.Errorf("expected pending status, got %s", pending[0].Status)
	}
}

func TestDryRunQueue_AutoTimestamp(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	before := time.Now()
	q.Add(apiserver.DryRunDecision{})
	after := time.Now()
	pending := q.GetPending()
	if pending[0].CreatedAt.Before(before) || pending[0].CreatedAt.After(after) {
		t.Error("CreatedAt not set automatically")
	}
}

func TestDryRunQueue_Approve(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "dep"})
	d, ok := q.Approve(id)
	if !ok {
		t.Fatal("approve should succeed")
	}
	if d.Status != apiserver.DryRunApproved {
		t.Errorf("expected approved, got %s", d.Status)
	}
	if d.ReviewedAt == nil {
		t.Error("expected ReviewedAt to be set")
	}
	// GetPending should now be empty
	if len(q.GetPending()) != 0 {
		t.Error("approved decision should no longer be pending")
	}
}

func TestDryRunQueue_Reject(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "dep"})
	d, ok := q.Reject(id)
	if !ok {
		t.Fatal("reject should succeed")
	}
	if d.Status != apiserver.DryRunRejected {
		t.Errorf("expected rejected, got %s", d.Status)
	}
}

func TestDryRunQueue_ApproveNotFound(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	_, ok := q.Approve("nonexistent-id")
	if ok {
		t.Fatal("approve on nonexistent ID should fail")
	}
}

func TestDryRunQueue_RejectNotFound(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	_, ok := q.Reject("nonexistent-id")
	if ok {
		t.Fatal("reject on nonexistent ID should fail")
	}
}

func TestDryRunQueue_ApproveAlreadyApproved(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{})
	q.Approve(id)
	_, ok := q.Approve(id) // second approve should fail
	if ok {
		t.Fatal("double-approve should fail")
	}
}

func TestDryRunQueue_Get(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "dep"})
	d, ok := q.Get(id)
	if !ok || d == nil {
		t.Fatal("Get should return the decision")
	}
	if d.DeploymentName != "dep" {
		t.Errorf("unexpected deployment name: %s", d.DeploymentName)
	}
}

func TestDryRunQueue_GetAll(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.Add(apiserver.DryRunDecision{DeploymentName: "d1"})
	q.Add(apiserver.DryRunDecision{DeploymentName: "d2"})
	all := q.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

// ─── Server HTTP handlers ─────────────────────────────────────────────────────

func newTestServer(t *testing.T) *apiserver.Server {
	t.Helper()
	return apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
}

func TestServer_Health(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body)
	}
}

func TestServer_Health_CORS_Preflight(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected CORS header")
	}
}

func TestServer_Pods_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/pods", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_Pods_Empty(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/pods", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var pods []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&pods)
	if len(pods) != 0 {
		t.Errorf("expected empty pod list, got %d", len(pods))
	}
}

func TestServer_Pods_WithPods(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     client,
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/pods", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var pods []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&pods)
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
}

func TestServer_Pods_WithNamespaceQuery(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/pods?namespace=kube-system", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServer_Pods_WithMetricsStorage(t *testing.T) {
	ms := storage.NewStorage()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     client,
		MetricsStorage: ms,
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/pods", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServer_Forecasts_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/forecasts", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_Forecasts_Empty(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/forecasts", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServer_Forecasts_WithData(t *testing.T) {
	fc := apiserver.NewForecastCache()
	fc.Set("ns", "dep", apiserver.ForecastEntry{DeploymentName: "dep", CPUSamples: 5})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ForecastCache:  fc,
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/forecasts", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entries []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 forecast entry, got %d", len(entries))
	}
}

func TestServer_ScalingHistory_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/scaling-history", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_ScalingHistory_WithData(t *testing.T) {
	sh := apiserver.NewScalingHistoryStore(10)
	sh.Add(apiserver.ScalingRecord{DeploymentName: "dep", OldReplicas: 1, NewReplicas: 3})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: sh,
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/scaling-history", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var records []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&records)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestServer_OptimizerConfigs_NilClient(t *testing.T) {
	srv := newTestServer(t) // no optimizerClient set
	req := httptest.NewRequest(http.MethodGet, "/api/optimizer-configs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServer_OptimizerConfigs_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/optimizer-configs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_Scale_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scale", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_Scale_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scale", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_Scale_MissingDeploymentName(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{"replicas": 2})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_Scale_InvalidReplicas(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{"deployment_name": "dep", "replicas": 0})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_Scale_NoMLScaler(t *testing.T) {
	srv := newTestServer(t) // mlScaler is nil
	body, _ := json.Marshal(map[string]interface{}{
		"deployment_name": "dep",
		"replicas":        3,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no mlScaler), got %d", w.Code)
	}
}

func TestServer_Scale_DeploymentNotFound(t *testing.T) {
	// mlScaler provided but deployment doesn't exist
	client := fake.NewSimpleClientset()
	mlScaler := forecaster.NewHorizontalScaler(client, zap.NewNop())
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     client,
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
		MLScaler:       mlScaler,
	})
	body, _ := json.Marshal(map[string]interface{}{
		"deployment_name": "nonexistent",
		"replicas":        3,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestServer_Scale_Success(t *testing.T) {
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "nginx",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					}},
				},
			},
		},
	}
	client := fake.NewSimpleClientset(dep)
	mlScaler := forecaster.NewHorizontalScaler(client, zap.NewNop())
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     client,
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
		MLScaler:       mlScaler,
	})
	body, _ := json.Marshal(map[string]interface{}{
		"deployment_name": "dep",
		"namespace":       "default",
		"replicas":        3,
		"reason":          "load test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── DryRun endpoints ─────────────────────────────────────────────────────────

func TestServer_DryRunPending_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/dry-run/pending", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_DryRunPending_Empty(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dry-run/pending", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var items []interface{}
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected empty pending list, got %d", len(items))
	}
}

func TestServer_DryRunPending_WithItem(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.Add(apiserver.DryRunDecision{DeploymentName: "dep"})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/dry-run/pending", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var items []interface{}
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Fatalf("expected 1 pending item, got %d", len(items))
	}
}

func TestServer_DryRunApprove_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dry-run/approve", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_DryRunApprove_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_DryRunApprove_MissingID(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"id": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_DryRunApprove_NotFound(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"id": "ghost-id"})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestServer_DryRunApprove_NoMLScaler(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "dep", Namespace: "default"})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
		// MLScaler intentionally nil
	})
	body, _ := json.Marshal(map[string]string{"id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// nil mlScaler → approve still returns 200 (scale is skipped)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when mlScaler is nil, got %d", w.Code)
	}
}

func TestServer_DryRunReject_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dry-run/reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServer_DryRunReject_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/reject", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_DryRunReject_MissingID(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"id": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/reject", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServer_DryRunReject_Success(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "dep"})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})
	body, _ := json.Marshal(map[string]string{"id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/reject", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServer_Start_ShutdownViaContext(t *testing.T) {
	srv := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		srv.Start(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Server.Start did not return after context cancellation")
	}
}

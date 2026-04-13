package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/apiserver"

	"go.uber.org/zap"
	"intelligent-cluster-optimizer/pkg/forecaster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newIntServer(t *testing.T) *apiserver.Server {
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

// ─── ScalingHistoryStore integration ─────────────────────────────────────────

func TestIntScalingHistory_RingBuffer(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(3)
	for i := 0; i < 7; i++ {
		s.Add(apiserver.ScalingRecord{DeploymentName: "dep", NewReplicas: int32(i + 1)})
	}
	all := s.GetAll()
	if len(all) > 3 {
		t.Fatalf("expected cap at 3, got %d", len(all))
	}
	// Newest should be first
	if all[0].NewReplicas != 7 {
		t.Errorf("expected newest replica count 7, got %d", all[0].NewReplicas)
	}
}

func TestIntScalingHistory_AutoID_Unique(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(100)
	ids := map[string]bool{}
	for i := 0; i < 20; i++ {
		s.Add(apiserver.ScalingRecord{DeploymentName: "dep"})
	}
	for _, r := range s.GetAll() {
		if ids[r.ID] {
			t.Fatalf("duplicate ID: %s", r.ID)
		}
		ids[r.ID] = true
	}
}

func TestIntScalingHistory_PreserveFields(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(10)
	s.Add(apiserver.ScalingRecord{
		ID:             "custom-id",
		Namespace:      "prod",
		DeploymentName: "web",
		OldReplicas:    2,
		NewReplicas:    5,
		Reason:         "load spike",
		PeakCPU:        85.5,
		Applied:        true,
	})
	all := s.GetAll()
	r := all[0]
	if r.ID != "custom-id" {
		t.Errorf("ID not preserved: %s", r.ID)
	}
	if r.PeakCPU != 85.5 {
		t.Errorf("PeakCPU not preserved: %f", r.PeakCPU)
	}
}

// ─── ForecastCache integration ────────────────────────────────────────────────

func TestIntForecastCache_MultipleNamespaces(t *testing.T) {
	c := apiserver.NewForecastCache()
	c.Set("ns1", "dep", apiserver.ForecastEntry{DeploymentName: "dep", Namespace: "ns1"})
	c.Set("ns2", "dep", apiserver.ForecastEntry{DeploymentName: "dep", Namespace: "ns2"})
	c.Set("ns1", "dep2", apiserver.ForecastEntry{DeploymentName: "dep2", Namespace: "ns1"})
	all := c.GetAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
}

func TestIntForecastCache_WithDecision(t *testing.T) {
	c := apiserver.NewForecastCache()
	decision := forecaster.ScaleDecision{DesiredReplicas: 5, ScaleUp: true}
	c.Set("ns", "dep", apiserver.ForecastEntry{
		DeploymentName: "dep",
		Decision:       &decision,
		InferenceMs:    12.5,
		CPUSamples:     100,
	})
	all := c.GetAll()
	if all[0].Decision == nil {
		t.Error("decision should be preserved")
	}
	if all[0].Decision.DesiredReplicas != 5 {
		t.Errorf("expected 5 desired replicas, got %d", all[0].Decision.DesiredReplicas)
	}
}

// ─── DryRunQueue integration ──────────────────────────────────────────────────

func TestIntDryRunQueue_LifeCycle(t *testing.T) {
	q := apiserver.NewDryRunQueue()

	// Add two decisions
	id1 := q.Add(apiserver.DryRunDecision{DeploymentName: "dep1", DesiredReplicas: 3})
	id2 := q.Add(apiserver.DryRunDecision{DeploymentName: "dep2", DesiredReplicas: 5})

	pending := q.GetPending()
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}

	// Approve first
	d1, ok := q.Approve(id1)
	if !ok || d1.Status != apiserver.DryRunApproved {
		t.Fatal("approve failed")
	}

	// Reject second
	d2, ok := q.Reject(id2)
	if !ok || d2.Status != apiserver.DryRunRejected {
		t.Fatal("reject failed")
	}

	// Pending should be empty now
	if len(q.GetPending()) != 0 {
		t.Error("expected 0 pending after approve/reject")
	}

	// GetAll should have both
	all := q.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 in GetAll, got %d", len(all))
	}
}

func TestIntDryRunQueue_Get(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "dep", Reason: "test"})
	d, ok := q.Get(id)
	if !ok || d.Reason != "test" {
		t.Fatalf("Get failed or wrong data: ok=%v reason=%s", ok, d.Reason)
	}
}

func TestIntDryRunQueue_Get_Missing(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	_, ok := q.Get("no-such-id")
	if ok {
		t.Fatal("expected false for missing id")
	}
}

func TestIntDryRunQueue_DoubleApprove(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{})
	q.Approve(id)
	_, ok := q.Approve(id)
	if ok {
		t.Fatal("double approve should fail")
	}
}

func TestIntDryRunQueue_RejectAfterApprove(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{})
	q.Approve(id)
	_, ok := q.Reject(id)
	if ok {
		t.Fatal("reject after approve should fail")
	}
}

// ─── Server handler integration ───────────────────────────────────────────────

func TestIntServer_Health(t *testing.T) {
	srv := newIntServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestIntServer_CORS_Options(t *testing.T) {
	srv := newIntServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("missing CORS header")
	}
}

func TestIntServer_CORS_NoOrigin(t *testing.T) {
	srv := newIntServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// No Origin header → should default to *
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("expected * origin when none set, got %s", origin)
	}
}

func TestIntServer_Pods_MethodNotAllowed(t *testing.T) {
	srv := newIntServer(t)
	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPut} {
		req := httptest.NewRequest(method, "/api/pods", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, w.Code)
		}
	}
}

func TestIntServer_Pods_WithPodAndTerminating(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "running-pod", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "terminating-pod",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
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
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(pods))
	}
	// Check Terminating status
	for _, p := range pods {
		if p["name"] == "terminating-pod" && p["status"] != "Terminating" {
			t.Errorf("expected Terminating, got %v", p["status"])
		}
	}
}

func TestIntServer_Pods_RestartCount(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", RestartCount: 5},
				{Name: "sidecar", RestartCount: 3},
			},
		},
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
	var pods []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&pods)
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	rc := pods[0]["restart_count"].(float64)
	if rc != 8 {
		t.Errorf("expected restart count 8, got %v", rc)
	}
}

func TestIntServer_Forecasts_WithData(t *testing.T) {
	fc := apiserver.NewForecastCache()
	fc.Set("ns", "dep1", apiserver.ForecastEntry{DeploymentName: "dep1", CPUSamples: 50})
	fc.Set("ns", "dep2", apiserver.ForecastEntry{DeploymentName: "dep2", CPUSamples: 30})
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
	var entries []interface{}
	json.NewDecoder(w.Body).Decode(&entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 forecast entries, got %d", len(entries))
	}
}

func TestIntServer_ScalingHistory_Ordering(t *testing.T) {
	sh := apiserver.NewScalingHistoryStore(10)
	sh.Add(apiserver.ScalingRecord{DeploymentName: "first", NewReplicas: 2})
	time.Sleep(time.Millisecond)
	sh.Add(apiserver.ScalingRecord{DeploymentName: "second", NewReplicas: 4})
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
	var records []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&records)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	// Newest first
	if records[0]["deployment_name"] != "second" {
		t.Errorf("expected second first (newest), got %v", records[0]["deployment_name"])
	}
}

func TestIntServer_Scale_NoNamespace_UsesDefault(t *testing.T) {
	replicas := int32(2)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				}}},
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
	// No namespace in body → should use server default
	body, _ := json.Marshal(map[string]interface{}{
		"deployment_name": "dep",
		"replicas":        5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIntServer_Scale_DefaultReason(t *testing.T) {
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				}}},
			},
		},
	}
	client := fake.NewSimpleClientset(dep)
	mlScaler := forecaster.NewHorizontalScaler(client, zap.NewNop())
	sh := apiserver.NewScalingHistoryStore(10)
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     client,
		ScalingHistory: sh,
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
		MLScaler:       mlScaler,
	})
	// No reason → should default to "Manual"
	body, _ := json.Marshal(map[string]interface{}{
		"deployment_name": "dep",
		"namespace":       "default",
		"replicas":        3,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Check history recorded with default reason
	all := sh.GetAll()
	if len(all) == 0 {
		t.Fatal("expected scaling history record")
	}
	if all[0].Reason != "Manual" {
		t.Errorf("expected reason=Manual, got %s", all[0].Reason)
	}
}

func TestIntServer_DryRunApprove_WithMLScaler(t *testing.T) {
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				}}},
			},
		},
	}
	client := fake.NewSimpleClientset(dep)
	mlScaler := forecaster.NewHorizontalScaler(client, zap.NewNop())
	q := apiserver.NewDryRunQueue()
	sh := apiserver.NewScalingHistoryStore(10)
	id := q.Add(apiserver.DryRunDecision{
		DeploymentName:  "dep",
		Namespace:       "default",
		CurrentReplicas: 1,
		DesiredReplicas: 3,
		Reason:          "ml-forecast",
		Decision:        forecaster.ScaleDecision{DesiredReplicas: 3, ScaleUp: true},
	})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     client,
		ScalingHistory: sh,
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
		MLScaler:       mlScaler,
	})
	body, _ := json.Marshal(map[string]string{"id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify history was recorded
	if len(sh.GetAll()) == 0 {
		t.Error("expected scaling history record after approve")
	}
}

func TestIntServer_DryRunReject_Full(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id1 := q.Add(apiserver.DryRunDecision{DeploymentName: "dep1"})
	id2 := q.Add(apiserver.DryRunDecision{DeploymentName: "dep2"})

	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(10),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})

	for _, id := range []string{id1, id2} {
		body, _ := json.Marshal(map[string]string{"id": id})
		req := httptest.NewRequest(http.MethodPost, "/api/dry-run/reject", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for reject %s, got %d", id, w.Code)
		}
	}
	if len(q.GetPending()) != 0 {
		t.Error("expected all decisions rejected")
	}
}

func TestIntServer_Scale_NegativeReplicas(t *testing.T) {
	srv := newIntServer(t)
	body, _ := json.Marshal(map[string]interface{}{"deployment_name": "dep", "replicas": -1})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative replicas, got %d", w.Code)
	}
}

func TestIntServer_DryRunApprove_NotFound(t *testing.T) {
	srv := newIntServer(t)
	body, _ := json.Marshal(map[string]string{"id": "ghost"})
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestIntServer_ScaleDownScenario(t *testing.T) {
	replicas := int32(5)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				}}},
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
		"deployment_name": "web",
		"namespace":       "default",
		"replicas":        2, // scale DOWN
		"reason":          "off-peak",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scale", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["old_replicas"].(float64) != 5 {
		t.Errorf("expected old_replicas=5, got %v", resp["old_replicas"])
	}
	if resp["new_replicas"].(float64) != 2 {
		t.Errorf("expected new_replicas=2, got %v", resp["new_replicas"])
	}
}

func TestIntServer_InvalidJSON_AllEndpoints(t *testing.T) {
	srv := newIntServer(t)
	endpoints := []string{"/api/scale", "/api/dry-run/approve", "/api/dry-run/reject"}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodPost, ep, strings.NewReader("not-json"))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400 for invalid JSON, got %d", ep, w.Code)
		}
	}
}

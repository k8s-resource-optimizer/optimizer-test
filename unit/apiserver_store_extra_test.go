package unit_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/apiserver"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

// ── ScalingHistoryStore extra ─────────────────────────────────────────────────

func TestScalingHistoryStore_GetSnapshotJSON(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(100)
	s.Add(apiserver.ScalingRecord{
		Namespace:      "default",
		DeploymentName: "app",
		Reason:         "ML",
		Applied:        true,
	})
	data, err := s.GetSnapshotJSON()
	if err != nil {
		t.Fatalf("GetSnapshotJSON: %v", err)
	}
	var records []apiserver.ScalingRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}
}

func TestScalingHistoryStore_RestoreFromJSON(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(100)
	s.Add(apiserver.ScalingRecord{Namespace: "ns", DeploymentName: "app", Applied: true})
	data, _ := s.GetSnapshotJSON()

	s2 := apiserver.NewScalingHistoryStore(100)
	if err := s2.RestoreFromJSON(data); err != nil {
		t.Fatalf("RestoreFromJSON: %v", err)
	}
	if len(s2.GetAll()) != 1 {
		t.Errorf("expected 1 record after restore, got %d", len(s2.GetAll()))
	}
}

func TestScalingHistoryStore_RestoreFromJSON_Invalid(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(100)
	if err := s.RestoreFromJSON([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ── DryRunQueue extra ─────────────────────────────────────────────────────────

func TestDryRunQueue_RevertToPending(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{
		Namespace: "default", DeploymentName: "app",
		ScalingType: "horizontal", Reason: "test",
	})
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	d, ok := q.Approve(id)
	if !ok {
		t.Fatal("approve failed")
	}
	if d.Status != apiserver.DryRunApproved {
		t.Errorf("expected approved, got %s", d.Status)
	}

	q.RevertToPending(id)

	got, ok := q.Get(id)
	if !ok {
		t.Fatal("get after revert failed")
	}
	if got.Status != apiserver.DryRunPending {
		t.Errorf("expected pending after revert, got %s", got.Status)
	}
	if got.ReviewedAt != nil {
		t.Error("ReviewedAt should be nil after revert")
	}
}

func TestDryRunQueue_RevertToPending_NonExistent(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.RevertToPending("nonexistent-id")
}

func TestDryRunQueue_Add_CooldownPreventsDouble(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	d := apiserver.DryRunDecision{
		Namespace: "ns", DeploymentName: "app",
		ContainerName: "c", ScalingType: "vertical", Reason: "test",
	}
	id1 := q.Add(d)
	if id1 == "" {
		t.Fatal("first add should succeed")
	}
	q.Approve(id1)

	id2 := q.Add(d)
	if id2 != "" {
		t.Errorf("second add within cooldown should return empty, got %s", id2)
	}
}

// ── ForecastCache extra ───────────────────────────────────────────────────────

func TestForecastCache_Set_SetsTimestamp(t *testing.T) {
	c := apiserver.NewForecastCache()
	before := time.Now()
	c.Set("ns", "app", apiserver.ForecastEntry{})
	all := c.GetAll()
	if len(all) != 1 {
		t.Fatal("expected 1 entry")
	}
	if all[0].Timestamp.Before(before) {
		t.Error("timestamp should be set to current time")
	}
}

// ── /api/metrics-history endpoint ────────────────────────────────────────────

func newMetricsServer() (*apiserver.Server, *storage.InMemoryStorage) {
	store := storage.NewStorage()
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		MetricsStorage: store,
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	return srv, store
}

func metricsGet(srv *apiserver.Server, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func metricsPost(srv *apiserver.Server, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestHandleMetricsHistory_MissingParams(t *testing.T) {
	srv, _ := newMetricsServer()
	w := metricsGet(srv, "/api/metrics-history")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMetricsHistory_MissingDeployment(t *testing.T) {
	srv, _ := newMetricsServer()
	w := metricsGet(srv, "/api/metrics-history?namespace=default")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMetricsHistory_MissingNamespace(t *testing.T) {
	srv, _ := newMetricsServer()
	w := metricsGet(srv, "/api/metrics-history?deployment=my-app")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMetricsHistory_MethodNotAllowed(t *testing.T) {
	srv, _ := newMetricsServer()
	w := metricsPost(srv, "/api/metrics-history")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleMetricsHistory_EmptyStorage(t *testing.T) {
	srv, _ := newMetricsServer()
	w := metricsGet(srv, "/api/metrics-history?namespace=default&deployment=my-app")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var points []apiserver.MetricsDataPoint
	if err := json.NewDecoder(w.Body).Decode(&points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("expected empty array, got %d points", len(points))
	}
}

func TestHandleMetricsHistory_ReturnsPoints(t *testing.T) {
	srv, store := newMetricsServer()
	now := time.Now()
	store.Add(models.PodMetric{
		PodName:   "my-app-abc-xyz",
		Namespace: "default",
		Timestamp: now,
		Containers: []models.ContainerMetric{
			{ContainerName: "main", UsageCPU: 100, UsageMemory: 128 * 1024 * 1024},
		},
	})
	w := metricsGet(srv, "/api/metrics-history?namespace=default&deployment=my-app")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var points []apiserver.MetricsDataPoint
	if err := json.NewDecoder(w.Body).Decode(&points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) == 0 {
		t.Error("expected at least one data point")
	}
}

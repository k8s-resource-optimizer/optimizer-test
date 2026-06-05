package unit_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/reports"
)

func makeWorkloadRec(ns, name, kind, container string, curCPU, recCPU, curMem, recMem int64, conf float64) recommendation.WorkloadRecommendation {
	return recommendation.WorkloadRecommendation{
		Namespace:    ns,
		WorkloadName: name,
		WorkloadKind: kind,
		GeneratedAt:  time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Containers: []recommendation.ContainerRecommendation{
			{
				ContainerName:     container,
				CurrentCPU:        curCPU,
				RecommendedCPU:    recCPU,
				CurrentMemory:     curMem,
				RecommendedMemory: recMem,
				Confidence:        conf,
			},
		},
	}
}

func TestGenerateCostReport_EmptyRecommendations(t *testing.T) {
	r := reports.GenerateCostReport("test-cluster", nil)
	if r == nil {
		t.Fatal("expected non-nil report")
	}
	if r.ClusterName != "test-cluster" {
		t.Errorf("expected cluster name 'test-cluster', got %q", r.ClusterName)
	}
}

func TestGenerateCostReport_OverProvisioned(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("default", "stress-cyclic", "Deployment", "app",
			400, 150, 256*1024*1024, 64*1024*1024, 74.0),
		makeWorkloadRec("default", "stress-master", "Deployment", "app",
			800, 200, 512*1024*1024, 128*1024*1024, 85.0),
	}
	r := reports.GenerateCostReport("gke-cluster", recs)
	if r == nil {
		t.Fatal("expected non-nil report")
	}
	if r.ClusterName != "gke-cluster" {
		t.Errorf("unexpected cluster name: %q", r.ClusterName)
	}
}

func TestGenerateCostReport_UnderProvisioned(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("production", "api-server", "Deployment", "app",
			100, 400, 64*1024*1024, 256*1024*1024, 90.0),
	}
	r := reports.GenerateCostReport("prod-cluster", recs)
	if r == nil {
		t.Fatal("expected non-nil report for under-provisioned workload")
	}
}

func TestGenerateCostReport_Summary_Fields(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("default", "wl1", "Deployment", "app", 400, 100, 256*1024*1024, 64*1024*1024, 80.0),
		makeWorkloadRec("default", "wl2", "StatefulSet", "db", 100, 400, 64*1024*1024, 256*1024*1024, 75.0),
		makeWorkloadRec("staging", "wl3", "Deployment", "app", 200, 200, 128*1024*1024, 128*1024*1024, 95.0),
	}
	r := reports.GenerateCostReport("mixed-cluster", recs)
	if r.Summary.TotalWorkloads == 0 {
		t.Error("expected non-zero TotalWorkloads in summary")
	}
}

func TestCostReport_ExportJSON(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("default", "web", "Deployment", "app", 400, 150, 256*1024*1024, 64*1024*1024, 80.0),
	}
	r := reports.GenerateCostReport("json-test", recs)

	var buf bytes.Buffer
	if err := r.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON error: %v", err)
	}
	if !strings.Contains(buf.String(), "json-test") {
		t.Error("JSON output should contain cluster name")
	}
}

func TestCostReport_ExportCSV(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("default", "web", "Deployment", "app", 400, 150, 256*1024*1024, 64*1024*1024, 80.0),
		makeWorkloadRec("default", "api", "Deployment", "server", 600, 200, 512*1024*1024, 128*1024*1024, 85.0),
	}
	r := reports.GenerateCostReport("csv-test", recs)

	var buf bytes.Buffer
	if err := r.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty CSV output")
	}
}

func TestCostReport_ExportHTML(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("production", "stress-cyclic", "Deployment", "app",
			400, 150, 256*1024*1024, 64*1024*1024, 74.0),
	}
	r := reports.GenerateCostReport("html-test", recs)

	var buf bytes.Buffer
	if err := r.ExportHTML(&buf); err != nil {
		t.Fatalf("ExportHTML error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "<html") && !strings.Contains(html, "<!DOCTYPE") && !strings.Contains(html, "html-test") {
		t.Error("ExportHTML should return HTML content containing the cluster name")
	}
}

func TestCostReport_ExportJSON_EmptyReport(t *testing.T) {
	r := reports.GenerateCostReport("empty-cluster", nil)
	var buf bytes.Buffer
	if err := r.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON on empty report error: %v", err)
	}
}

func TestCostReport_ExportCSV_EmptyReport(t *testing.T) {
	r := reports.GenerateCostReport("empty-cluster", nil)
	var buf bytes.Buffer
	if err := r.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV on empty report error: %v", err)
	}
}

func TestCostReport_ExportHTML_EmptyReport(t *testing.T) {
	r := reports.GenerateCostReport("empty-cluster", nil)
	var buf bytes.Buffer
	if err := r.ExportHTML(&buf); err != nil {
		t.Fatalf("ExportHTML on empty report error: %v", err)
	}
}

func TestCostReport_PotentialSavings_OverProvisioned(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		makeWorkloadRec("default", "fat-pod", "Deployment", "app",
			1000, 100, 1024*1024*1024, 128*1024*1024, 90.0),
	}
	r := reports.GenerateCostReport("savings-test", recs)
	if r.PotentialSavings < 0 {
		t.Error("PotentialSavings should not be negative for over-provisioned workload")
	}
}

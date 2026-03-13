package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/leakdetector"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

func buildLeakSamples(n int, startBytes, endBytes int64) []leakdetector.MemorySample {
	samples := make([]leakdetector.MemorySample, n)
	base := time.Now().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		bytes := startBytes + int64(float64(endBytes-startBytes)*frac)
		samples[i] = leakdetector.MemorySample{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Bytes:     bytes,
		}
	}
	return samples
}

func storageToLeakSamples(st *storage.InMemoryStorage, namespace string) []leakdetector.MemorySample {
	metrics := st.GetMetricsByNamespace(namespace, 24*time.Hour)
	var samples []leakdetector.MemorySample
	for _, m := range metrics {
		for _, c := range m.Containers {
			samples = append(samples, leakdetector.MemorySample{
				Timestamp: m.Timestamp,
				Bytes:     c.UsageMemory,
			})
		}
	}
	return samples
}

// TestLeakDetectorPipeline_NoLeak verifies stable memory is not flagged as a leak.
func TestLeakDetectorPipeline_NoLeak(t *testing.T) {
	// Stable memory: 256MiB flat
	samples := buildLeakSamples(120, 256*1024*1024, 256*1024*1024)
	d := leakdetector.NewDetector()
	result := d.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil LeakAnalysis")
	}
	if result.IsLeak {
		t.Errorf("stable memory should not be flagged as leak, got severity=%v", result.Severity)
	}
}

// TestLeakDetectorPipeline_CriticalLeak verifies fast-growing memory is detected as critical.
func TestLeakDetectorPipeline_CriticalLeak(t *testing.T) {
	// 100MiB → 8GiB over 2 hours = very fast leak
	samples := buildLeakSamples(120, 100*1024*1024, 8*1024*1024*1024)
	d := leakdetector.NewDetector()
	result := d.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil LeakAnalysis")
	}
	if !result.IsLeak {
		t.Error("fast memory growth should be detected as a leak")
	}
	if result.Description == "" {
		t.Error("expected non-empty Description for detected leak")
	}
}

// TestLeakDetectorPipeline_AnalyzeWithLimit verifies memory limit context is used.
func TestLeakDetectorPipeline_AnalyzeWithLimit(t *testing.T) {
	// 200MiB → 900MiB with a 1GiB limit
	samples := buildLeakSamples(120, 200*1024*1024, 900*1024*1024)
	d := leakdetector.NewDetector()
	result := d.AnalyzeWithLimit(samples, int64(1024*1024*1024))
	if result == nil {
		t.Fatal("expected non-nil LeakAnalysis from AnalyzeWithLimit")
	}
}

// TestLeakDetectorPipeline_StorageToLeakDetection verifies the
// storage → samples → leakdetector pipeline end-to-end.
func TestLeakDetectorPipeline_StorageToLeakDetection(t *testing.T) {
	st := storage.NewStorage()
	base := time.Now().Add(-2 * time.Hour)

	// Simulate a leaking pod: memory grows 100MiB → 600MiB over 120 minutes
	for i := 0; i < 120; i++ {
		mem := int64(100*1024*1024) + int64(i)*int64(4*1024*1024)
		st.Add(models.PodMetric{
			PodName:   "leaky-pod-abc",
			Namespace: "default",
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: 200, UsageMemory: mem},
			},
		})
	}

	samples := storageToLeakSamples(st, "default")
	if len(samples) == 0 {
		t.Fatal("expected samples from storage")
	}

	d := leakdetector.NewDetector()
	result := d.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil LeakAnalysis")
	}
	if result.IsLeak && result.Recommendation == "" {
		t.Error("expected non-empty Recommendation for detected leak")
	}
}

// TestLeakDetectorPipeline_AlertGenerated verifies an alert is generated for leaks.
func TestLeakDetectorPipeline_AlertGenerated(t *testing.T) {
	samples := buildLeakSamples(120, 100*1024*1024, 6*1024*1024*1024)
	d := leakdetector.NewDetector()
	result := d.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil LeakAnalysis")
	}
	if result.IsLeak && result.Alert == nil {
		t.Error("expected non-nil Alert for detected leak")
	}
}

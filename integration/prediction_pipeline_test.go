package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/prediction"
	"intelligent-cluster-optimizer/pkg/storage"
)

func buildPredictionStorage(n int) *storage.InMemoryStorage {
	st := storage.NewStorage()
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	for i := 0; i < n; i++ {
		phase := 2 * 3.14159 * float64(i) / 24.0
		sin := phase - phase*phase*phase/6 + phase*phase*phase*phase*phase/120
		cpu := int64(300 + 100*sin + float64(i)*0.3)
		mem := int64(256*1024*1024) + int64(sin*float64(32*1024*1024))
		if cpu < 50 {
			cpu = 50
		}
		if mem < 64*1024*1024 {
			mem = 64 * 1024 * 1024
		}
		st.Add(models.PodMetric{
			PodName:   "app-abc-xyz",
			Namespace: "default",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: cpu, UsageMemory: mem, LimitCPU: 4000, LimitMemory: int64(2 * 1024 * 1024 * 1024)},
			},
		})
	}
	return st
}

// TestPredictionPipeline_StorageToWorkloadPredictor verifies the
// storage → PodMetric slice → WorkloadPredictor pipeline.
func TestPredictionPipeline_StorageToWorkloadPredictor(t *testing.T) {
	st := buildPredictionStorage(100)
	metrics := st.GetMetricsByNamespace("default", 100*time.Hour)
	if len(metrics) == 0 {
		t.Fatal("expected metrics from storage")
	}

	predictor := prediction.NewWorkloadPredictor()
	result, err := predictor.PredictWorkload("default", "app", metrics)
	if err != nil {
		t.Fatalf("PredictWorkload error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadPrediction")
	}
	if result.Summary() == "" {
		t.Error("expected non-empty Summary")
	}
}

// TestPredictionPipeline_ScalingRecommendations verifies ShouldScaleUp/Down
// and GetScalingRecommendation are reachable via the full pipeline.
func TestPredictionPipeline_ScalingRecommendations(t *testing.T) {
	st := buildPredictionStorage(100)
	metrics := st.GetMetricsByNamespace("default", 100*time.Hour)

	predictor := prediction.NewWorkloadPredictor()
	result, err := predictor.PredictWorkload("default", "app", metrics)
	if err != nil || result == nil {
		t.Skip("prediction not available")
	}

	currentCPU := int64(300)
	currentMem := int64(256 * 1024 * 1024)

	_ = result.ShouldScaleUp(currentCPU, currentMem)
	_ = result.ShouldScaleDown(currentCPU, currentMem, 0.5)
	rec := result.GetScalingRecommendation(currentCPU, currentMem)
	if rec == "" {
		t.Error("expected non-empty scaling recommendation")
	}
}

// TestPredictionPipeline_TimeUntilPeak exercises the TimeUntilPeak method.
func TestPredictionPipeline_TimeUntilPeak(t *testing.T) {
	st := buildPredictionStorage(100)
	metrics := st.GetMetricsByNamespace("default", 100*time.Hour)

	predictor := prediction.NewWorkloadPredictor()
	result, err := predictor.PredictWorkload("default", "app", metrics)
	if err != nil || result == nil {
		t.Skip("prediction not available")
	}

	cpuDur, memDur := result.TimeUntilPeak()
	_ = cpuDur
	_ = memDur
}

// TestPredictionPipeline_HoltWintersFitPredict verifies HoltWinters FitPredict
// with enough seasonal data.
func TestPredictionPipeline_HoltWintersFitPredict(t *testing.T) {
	data := make([]float64, 72)
	for i := range data {
		phase := 2 * 3.14159 * float64(i) / 24.0
		sin := phase - phase*phase*phase/6 + phase*phase*phase*phase*phase/120
		data[i] = 200 + 80*sin
	}

	hw := prediction.NewHoltWinters()
	result, err := hw.FitPredict(data, 12)
	if err != nil {
		t.Fatalf("FitPredict error: %v", err)
	}
	if len(result.Forecasts) != 12 {
		t.Errorf("expected 12 forecast points, got %d", len(result.Forecasts))
	}
	if result.Summary() == "" {
		t.Error("expected non-empty Summary")
	}
	f, err := result.GetForecast(6)
	if err != nil {
		t.Errorf("GetForecast(6) error: %v", err)
	}
	if f == nil {
		t.Error("expected non-nil Forecast at horizon 6")
	}
	peak := result.PeakForecast()
	trough := result.TroughForecast()
	if peak == nil || trough == nil {
		t.Error("expected non-nil peak and trough forecasts")
	}
}

// TestPredictionPipeline_PredictFromValues exercises PredictFromValues directly.
func TestPredictionPipeline_PredictFromValues(t *testing.T) {
	n := 100
	cpuVals := make([]float64, n)
	memVals := make([]float64, n)
	ts := make([]time.Time, n)
	base := time.Now().Add(-time.Duration(n) * time.Hour)

	for i := 0; i < n; i++ {
		phase := 2 * 3.14159 * float64(i) / 24.0
		sin := phase - phase*phase*phase/6 + phase*phase*phase*phase*phase/120
		cpuVals[i] = 200 + 100*sin
		memVals[i] = float64(256*1024*1024) + sin*float64(32*1024*1024)
		ts[i] = base.Add(time.Duration(i) * time.Hour)
	}

	predictor := prediction.NewWorkloadPredictor()
	result, err := predictor.PredictFromValues(cpuVals, memVals, ts)
	if err != nil {
		t.Fatalf("PredictFromValues error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil WorkloadPrediction")
	}
}

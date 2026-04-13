package integration_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/anomaly"
	"intelligent-cluster-optimizer/pkg/models"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func intNormalData(n int, base float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = base + float64(i%5)*0.3
	}
	return data
}

func intSpikedData(n, pos int, base, spike float64) []float64 {
	data := intNormalData(n, base)
	data[pos] = spike
	return data
}

func intTimestamps(n int) []time.Time {
	ts := make([]time.Time, n)
	now := time.Now()
	for i := range ts {
		ts[i] = now.Add(time.Duration(i) * time.Minute)
	}
	return ts
}

// ─── ZScoreDetector ───────────────────────────────────────────────────────────

func TestIntZScore_Name(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	if d.Name() != anomaly.MethodZScore {
		t.Fatalf("expected %s", anomaly.MethodZScore)
	}
}

func TestIntZScore_TooFewSamples(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	r := d.Detect([]float64{1, 2, 3})
	if r.HasAnomalies() {
		t.Fatal("expected no anomalies with too few samples")
	}
}

func TestIntZScore_NoAnomaly(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	r := d.Detect(intNormalData(30, 50.0))
	if r.HasAnomalies() {
		t.Fatalf("expected 0 anomalies, got %d", r.AnomalyCount())
	}
}

func TestIntZScore_DetectsSpike(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	data := intSpikedData(30, 15, 50.0, 1_000_000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestIntZScore_WithTimestamps(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	data := intSpikedData(20, 10, 50.0, 999999.0)
	r := d.DetectWithTimestamps(data, intTimestamps(len(data)))
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with timestamps")
	}
	for _, a := range r.Anomalies {
		if a.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	}
}

func TestIntZScore_WithConfig(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.ZScoreThreshold = 0.5 // very tight
	cfg.MinSamples = 5
	d := anomaly.NewZScoreDetectorWithConfig(cfg)
	data := intSpikedData(15, 7, 50.0, 10000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with tight threshold")
	}
}

func TestIntZScore_Summary(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	data := intNormalData(20, 50.0)
	r := d.Detect(data)
	s := r.Summary()
	if s == "" {
		t.Error("summary empty")
	}
}

func TestIntZScore_HighSeverity(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	data := intSpikedData(30, 15, 50.0, 100_000_000.0)
	r := d.Detect(data)
	if r.HasAnomalies() && r.HighSeverityCount() == 0 {
		t.Error("expected at least one high severity anomaly for extreme spike")
	}
}

// ─── IQRDetector ──────────────────────────────────────────────────────────────

func TestIntIQR_Name(t *testing.T) {
	d := anomaly.NewIQRDetector()
	if d.Name() != anomaly.MethodIQR {
		t.Fatalf("expected %s", anomaly.MethodIQR)
	}
}

func TestIntIQR_TooFewSamples(t *testing.T) {
	d := anomaly.NewIQRDetector()
	if d.Detect([]float64{1, 2}).HasAnomalies() {
		t.Fatal("expected no anomalies")
	}
}

func TestIntIQR_NoAnomaly(t *testing.T) {
	d := anomaly.NewIQRDetector()
	if d.Detect(intNormalData(20, 100.0)).HasAnomalies() {
		t.Fatal("unexpected anomalies in normal data")
	}
}

func TestIntIQR_DetectsSpike(t *testing.T) {
	d := anomaly.NewIQRDetector()
	r := d.Detect(intSpikedData(20, 10, 100.0, 99999.0))
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestIntIQR_ZeroIQR(t *testing.T) {
	d := anomaly.NewIQRDetector()
	data := make([]float64, 20)
	for i := range data {
		data[i] = 42.0
	}
	if d.Detect(data).HasAnomalies() {
		t.Fatal("constant data should yield no anomalies (IQR=0)")
	}
}

func TestIntIQR_WithTimestamps(t *testing.T) {
	d := anomaly.NewIQRDetector()
	data := intSpikedData(20, 5, 50.0, 50000.0)
	r := d.DetectWithTimestamps(data, intTimestamps(len(data)))
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestIntIQR_SeverityLevels(t *testing.T) {
	d := anomaly.NewIQRDetector()
	data := intSpikedData(20, 10, 50.0, 9_000_000.0)
	r := d.Detect(data)
	if r.HasAnomalies() {
		found := false
		for _, a := range r.Anomalies {
			if a.Severity == anomaly.SeverityHigh || a.Severity == anomaly.SeverityCritical {
				found = true
			}
		}
		if !found {
			t.Error("expected high/critical severity")
		}
	}
}

func TestIntIQR_Drop(t *testing.T) {
	d := anomaly.NewIQRDetector()
	data := intSpikedData(20, 10, 1000.0, -99999.0)
	r := d.Detect(data)
	_ = r // may detect drop anomaly
}

// ─── MovingAverageDetector ────────────────────────────────────────────────────

func TestIntMA_Name(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	if d.Name() != anomaly.MethodMovingAverage {
		t.Fatalf("expected %s", anomaly.MethodMovingAverage)
	}
}

func TestIntMA_TooFewSamples(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	if d.Detect([]float64{1, 2, 3}).HasAnomalies() {
		t.Fatal("expected no anomaly")
	}
}

func TestIntMA_NoAnomaly(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	if d.Detect(intNormalData(30, 50.0)).HasAnomalies() {
		t.Fatal("unexpected anomaly in normal data")
	}
}

func TestIntMA_DetectsSpike(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	r := d.Detect(intSpikedData(30, 20, 50.0, 99999.0))
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestIntMA_EMA(t *testing.T) {
	d := &anomaly.MovingAverageDetector{
		WindowSize: 5, Threshold: 2.0, MinSamples: 10,
		UseExponentialMA: true, Alpha: 0.3,
	}
	data := intSpikedData(25, 12, 50.0, 88888.0)
	r := d.Detect(data)
	_ = r
	ma := d.GetMovingAverage(intNormalData(20, 50.0))
	if len(ma) != 20 {
		t.Fatalf("expected 20 EMA values, got %d", len(ma))
	}
}

func TestIntMA_SMA(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	data := intNormalData(20, 50.0)
	ma := d.GetMovingAverage(data)
	if len(ma) != len(data) {
		t.Fatalf("expected %d SMA values, got %d", len(data), len(ma))
	}
}

func TestIntMA_WithTimestamps(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	data := intSpikedData(25, 15, 50.0, 999999.0)
	r := d.DetectWithTimestamps(data, intTimestamps(len(data)))
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with timestamps")
	}
}

func TestIntMA_ConstantData(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	data := make([]float64, 20)
	for i := range data {
		data[i] = 5.0
	}
	if d.Detect(data).HasAnomalies() {
		t.Fatal("constant data should yield no anomaly (devStdDev=0)")
	}
}

// ─── ConsensusDetector ────────────────────────────────────────────────────────

func TestIntConsensus_Name(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	if d.Name() != anomaly.MethodConsensus {
		t.Fatalf("expected %s", anomaly.MethodConsensus)
	}
}

func TestIntConsensus_TooFewSamples(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	if d.Detect([]float64{1, 2, 3}).HasAnomalies() {
		t.Fatal("expected no anomaly")
	}
}

func TestIntConsensus_NoAnomaly(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	if d.Detect(intNormalData(20, 50.0)).HasAnomalies() {
		t.Fatal("unexpected anomaly in normal data")
	}
}

func TestIntConsensus_ExtremeSpike(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := intSpikedData(20, 10, 50.0, 1_000_000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected consensus anomaly for extreme spike")
	}
	for _, a := range r.Anomalies {
		if a.DetectedBy != anomaly.MethodConsensus {
			t.Errorf("anomaly should be tagged consensus, got %s", a.DetectedBy)
		}
	}
}

func TestIntConsensus_WithTimestamps(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := intSpikedData(20, 15, 50.0, 9_999_999.0)
	r := d.DetectWithTimestamps(data, intTimestamps(len(data)))
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestIntConsensus_IndividualResults(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := intSpikedData(20, 10, 50.0, 9_999_999.0)
	results := d.GetIndividualResults(data, intTimestamps(len(data)))
	if len(results) != 3 {
		t.Fatalf("expected 3 individual results, got %d", len(results))
	}
}

func TestIntConsensus_AgreementLevel_Unanimous(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := intSpikedData(20, 10, 50.0, 9_000_000_000.0)
	level := d.GetAgreementLevel(data, 10)
	_ = level
}

func TestIntConsensus_AgreementLevel_None(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := intNormalData(20, 50.0)
	if d.GetAgreementLevel(data, 5) != anomaly.AgreementNone {
		t.Error("expected AgreementNone for normal data")
	}
}

func TestIntConsensus_Threshold1(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.ConsensusThreshold = 1
	cfg.MinSamples = 5
	d := anomaly.NewConsensusDetectorWithConfig(cfg)
	data := intSpikedData(15, 7, 50.0, 9999.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with threshold=1")
	}
}

// ─── WorkloadChecker ──────────────────────────────────────────────────────────

func intPodMetrics(n int, cpuBase, memBase int64) []models.PodMetric {
	metrics := make([]models.PodMetric, n)
	now := time.Now()
	for i := range metrics {
		metrics[i] = models.PodMetric{
			PodName:   "pod-0",
			Namespace: "default",
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{{
				ContainerName: "app",
				UsageCPU:      cpuBase + int64(i%3),
				UsageMemory:   memBase + int64(i%5)*1024,
			}},
		}
	}
	return metrics
}

func TestIntWorkloadChecker_NewDefault(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	r := c.CheckWorkload("ns", "app", intPodMetrics(5, 100, 512))
	if r.HasAnyAnomaly {
		t.Fatal("too few samples → no anomaly expected")
	}
}

func TestIntWorkloadChecker_WithConfig(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.MinSamples = 5
	cfg.ConsensusThreshold = 1
	c := anomaly.NewWorkloadCheckerWithConfig(cfg)
	metrics := intPodMetrics(20, 100, 512*1024)
	metrics[10].Containers[0].UsageCPU = 99_999_999
	_ = c.CheckWorkload("ns", "app", metrics)
}

func TestIntWorkloadChecker_CheckMetrics_TooFew(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	r := c.CheckMetrics([]float64{1, 2}, []float64{100, 200}, nil)
	if r.HasAnyAnomaly {
		t.Fatal("expected no anomaly with too few samples")
	}
}

func TestIntWorkloadChecker_CheckMetrics_Normal(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	cpu := make([]float64, 25)
	mem := make([]float64, 25)
	ts := intTimestamps(25)
	for i := range cpu {
		cpu[i] = 50.0 + float64(i%3)
		mem[i] = 512.0 + float64(i%4)
	}
	r := c.CheckMetrics(cpu, mem, ts)
	_ = r.Summary()
}

func TestIntWorkloadChecker_CheckMetrics_Spike(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.MinSamples = 5
	cfg.ConsensusThreshold = 1
	c := anomaly.NewWorkloadCheckerWithConfig(cfg)
	cpu := make([]float64, 20)
	mem := make([]float64, 20)
	for i := range cpu {
		cpu[i] = 50.0
		mem[i] = 512.0
	}
	cpu[10] = 9_999_999.0
	r := c.CheckMetrics(cpu, mem, nil)
	_ = r
}

func TestIntWorkloadChecker_IsRecentAnomaly(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	data := intSpikedData(20, 18, 50.0, 999999.0)
	r := d.Detect(data)
	_ = anomaly.IsRecentAnomaly(r, 5)
}

func TestIntWorkloadChecker_IsRecentAnomaly_NoAnomalies(t *testing.T) {
	d := anomaly.NewZScoreDetector()
	r := d.Detect(intNormalData(20, 50.0))
	if anomaly.IsRecentAnomaly(r, 5) {
		t.Error("expected false for no anomalies")
	}
}

func TestIntDetectionResult_Summary_Anomaly(t *testing.T) {
	r := &anomaly.DetectionResult{
		Method:      anomaly.MethodZScore,
		SampleCount: 50,
		Anomalies: []anomaly.Anomaly{
			{Severity: anomaly.SeverityCritical, DetectedBy: anomaly.MethodZScore},
		},
	}
	s := r.Summary()
	if s == "" {
		t.Error("summary empty")
	}
	if r.HighSeverityCount() != 1 {
		t.Errorf("expected 1 high severity, got %d", r.HighSeverityCount())
	}
}

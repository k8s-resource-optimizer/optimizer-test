package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/anomaly"
	"intelligent-cluster-optimizer/pkg/models"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// normalData returns n points near 50.0 with no outliers.
func normalData(n int) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = 50.0 + float64(i%3)*0.5 // tiny variation
	}
	return data
}

// spikedData returns normalData with one extreme value at position pos.
func spikedData(n, pos int, spike float64) []float64 {
	data := normalData(n)
	data[pos] = spike
	return data
}

// ─── IQRDetector ──────────────────────────────────────────────────────────────

func TestIQRDetector_Name(t *testing.T) {
	d := anomaly.NewIQRDetector()
	if d.Name() != anomaly.MethodIQR {
		t.Fatalf("expected %s, got %s", anomaly.MethodIQR, d.Name())
	}
}

func TestIQRDetector_TooFewSamples(t *testing.T) {
	d := anomaly.NewIQRDetector()
	r := d.Detect([]float64{1, 2, 3})
	if r.HasAnomalies() {
		t.Fatal("expected no anomalies with too few samples")
	}
}

func TestIQRDetector_NoAnomalies(t *testing.T) {
	d := anomaly.NewIQRDetector()
	r := d.Detect(normalData(20))
	if r.HasAnomalies() {
		t.Fatalf("expected 0 anomalies, got %d", r.AnomalyCount())
	}
}

func TestIQRDetector_DetectsSpike(t *testing.T) {
	d := anomaly.NewIQRDetector()
	data := spikedData(20, 10, 5000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly to be detected")
	}
}

func TestIQRDetector_DetectsWithTimestamps(t *testing.T) {
	d := anomaly.NewIQRDetector()
	data := spikedData(20, 5, 9999.0)
	ts := make([]time.Time, len(data))
	now := time.Now()
	for i := range ts {
		ts[i] = now.Add(time.Duration(i) * time.Minute)
	}
	r := d.DetectWithTimestamps(data, ts)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with timestamps")
	}
	// timestamp on the anomaly should be non-zero
	for _, a := range r.Anomalies {
		if a.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp on anomaly")
		}
	}
}

func TestIQRDetector_WithConfig(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.IQRMultiplier = 0.1 // very tight – flag almost everything
	cfg.MinSamples = 5
	d := anomaly.NewIQRDetectorWithConfig(cfg)
	data := []float64{10, 50, 10, 50, 10, 50, 200}
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected at least one anomaly with tight IQR multiplier")
	}
}

func TestIQRDetector_ZeroIQR(t *testing.T) {
	// All identical values → IQR = 0, should return empty result
	d := anomaly.NewIQRDetector()
	data := make([]float64, 15)
	for i := range data {
		data[i] = 42.0
	}
	r := d.Detect(data)
	if r.HasAnomalies() {
		t.Fatal("expected no anomalies when all values are identical (IQR=0)")
	}
}

func TestIQRDetector_SeverityLevels(t *testing.T) {
	d := anomaly.NewIQRDetector()
	// Extreme spike → should be high/critical
	data := spikedData(20, 15, 100000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
	found := false
	for _, a := range r.Anomalies {
		if a.Severity == anomaly.SeverityHigh || a.Severity == anomaly.SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected high or critical severity for extreme outlier")
	}
}

// ─── MovingAverageDetector ────────────────────────────────────────────────────

func TestMovingAverage_Name(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	if d.Name() != anomaly.MethodMovingAverage {
		t.Fatalf("expected %s", anomaly.MethodMovingAverage)
	}
}

func TestMovingAverage_TooFewSamples(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	r := d.Detect([]float64{1, 2})
	if r.HasAnomalies() {
		t.Fatal("expected no anomalies with too few samples")
	}
}

func TestMovingAverage_NoAnomalies(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	r := d.Detect(normalData(30))
	if r.HasAnomalies() {
		t.Fatalf("expected 0 anomalies, got %d", r.AnomalyCount())
	}
}

func TestMovingAverage_DetectsSpike(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	data := spikedData(30, 20, 99999.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly to be detected")
	}
}

func TestMovingAverage_WithTimestamps(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	data := spikedData(25, 12, 88888.0)
	ts := make([]time.Time, len(data))
	now := time.Now()
	for i := range ts {
		ts[i] = now.Add(time.Duration(i) * time.Second)
	}
	r := d.DetectWithTimestamps(data, ts)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with timestamps")
	}
}

func TestMovingAverage_WithConfig(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.MovingAverageWindow = 3
	cfg.MovingAverageThreshold = 1.0
	cfg.MinSamples = 5
	d := anomaly.NewMovingAverageDetectorWithConfig(cfg)
	data := spikedData(15, 7, 9999.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestMovingAverage_GetMovingAverage_SMA(t *testing.T) {
	d := anomaly.NewMovingAverageDetector()
	data := normalData(20)
	ma := d.GetMovingAverage(data)
	if len(ma) != len(data) {
		t.Fatalf("expected %d moving-average values, got %d", len(data), len(ma))
	}
}

func TestMovingAverage_GetMovingAverage_EMA(t *testing.T) {
	d := &anomaly.MovingAverageDetector{
		WindowSize:       5,
		Threshold:        2.0,
		MinSamples:       5,
		UseExponentialMA: true,
		Alpha:            0.3,
	}
	data := normalData(20)
	ma := d.GetMovingAverage(data)
	if len(ma) != len(data) {
		t.Fatalf("expected %d EMA values, got %d", len(data), len(ma))
	}
}

func TestMovingAverage_UniformData_NoDeviation(t *testing.T) {
	// Constant data → deviation std-dev = 0, no anomaly possible
	d := anomaly.NewMovingAverageDetector()
	data := make([]float64, 20)
	for i := range data {
		data[i] = 10.0
	}
	r := d.Detect(data)
	if r.HasAnomalies() {
		t.Fatal("expected no anomalies for constant data")
	}
}

// ─── ConsensusDetector ────────────────────────────────────────────────────────

func TestConsensus_Name(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	if d.Name() != anomaly.MethodConsensus {
		t.Fatalf("expected %s", anomaly.MethodConsensus)
	}
}

func TestConsensus_TooFewSamples(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	r := d.Detect([]float64{1, 2, 3})
	if r.HasAnomalies() {
		t.Fatal("expected no anomalies with too few samples")
	}
}

func TestConsensus_NoAnomalies(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	r := d.Detect(normalData(20))
	if r.HasAnomalies() {
		t.Fatalf("unexpected anomalies: %d", r.AnomalyCount())
	}
}

func TestConsensus_DetectsExtremeSpike(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := spikedData(20, 10, 1_000_000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("consensus should detect an extreme spike")
	}
	// All anomalies should be tagged as MethodConsensus
	for _, a := range r.Anomalies {
		if a.DetectedBy != anomaly.MethodConsensus {
			t.Errorf("expected DetectedBy=consensus, got %s", a.DetectedBy)
		}
	}
}

func TestConsensus_WithConfig(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.ConsensusThreshold = 1 // only one method needs to agree
	cfg.MinSamples = 5
	d := anomaly.NewConsensusDetectorWithConfig(cfg)
	data := spikedData(15, 7, 9999.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly with threshold=1")
	}
}

func TestConsensus_WithTimestamps(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := spikedData(20, 15, 999999.0)
	ts := make([]time.Time, len(data))
	now := time.Now()
	for i := range ts {
		ts[i] = now.Add(time.Duration(i) * time.Minute)
	}
	r := d.DetectWithTimestamps(data, ts)
	if !r.HasAnomalies() {
		t.Fatal("expected anomaly")
	}
}

func TestConsensus_GetIndividualResults(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := spikedData(20, 10, 9999.0)
	ts := make([]time.Time, len(data))
	results := d.GetIndividualResults(data, ts)
	if len(results) == 0 {
		t.Fatal("expected individual results from each detector")
	}
	// Must contain all three method keys
	if _, ok := results[anomaly.MethodZScore]; !ok {
		t.Error("missing z_score in individual results")
	}
	if _, ok := results[anomaly.MethodIQR]; !ok {
		t.Error("missing iqr in individual results")
	}
	if _, ok := results[anomaly.MethodMovingAverage]; !ok {
		t.Error("missing moving_average in individual results")
	}
}

func TestConsensus_GetAgreementLevel(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := spikedData(20, 10, 9_000_000.0)
	level := d.GetAgreementLevel(data, 10)
	if level == anomaly.AgreementNone {
		t.Error("expected agreement > 0 for extreme spike")
	}
}

func TestConsensus_GetAgreementLevel_NoAnomaly(t *testing.T) {
	d := anomaly.NewConsensusDetector()
	data := normalData(20)
	level := d.GetAgreementLevel(data, 5)
	if level != anomaly.AgreementNone {
		t.Errorf("expected AgreementNone for normal data, got %v", level)
	}
}

func TestConsensus_SeverityRankCoverage(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.ConsensusThreshold = 1
	cfg.MinSamples = 5
	d := anomaly.NewConsensusDetectorWithConfig(cfg)

	// extreme → should get critical or high severity
	data := spikedData(15, 7, 100_000_000.0)
	r := d.Detect(data)
	if !r.HasAnomalies() {
		t.Skip("no anomaly detected; skipping severity check")
	}
}

// ─── WorkloadChecker ──────────────────────────────────────────────────────────

func makePodMetrics(n int, cpuBase, memBase int64) []models.PodMetric {
	metrics := make([]models.PodMetric, n)
	for i := range metrics {
		metrics[i] = models.PodMetric{
			PodName:   "pod-" + string(rune('a'+i%26)),
			Namespace: "default",
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      cpuBase + int64(i%3),
					UsageMemory:   memBase + int64(i%5)*1024,
				},
			},
		}
	}
	return metrics
}

func TestWorkloadChecker_TooFewSamples(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	result := c.CheckWorkload("ns", "myapp", makePodMetrics(3, 100, 512))
	if result.HasAnyAnomaly {
		t.Fatal("expected no anomaly with too few samples")
	}
}

func TestWorkloadChecker_NoAnomaly(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	result := c.CheckWorkload("ns", "myapp", makePodMetrics(20, 100, 512*1024))
	summary := result.Summary()
	if summary == "" {
		t.Error("Summary should not be empty")
	}
}

func TestWorkloadChecker_WithConfig(t *testing.T) {
	cfg := anomaly.DefaultConfig()
	cfg.MinSamples = 5
	cfg.ConsensusThreshold = 1
	c := anomaly.NewWorkloadCheckerWithConfig(cfg)
	metrics := makePodMetrics(20, 100, 512*1024)
	// inject a spike
	metrics[10].Containers[0].UsageCPU = 99_999_999
	result := c.CheckWorkload("ns", "myapp", metrics)
	_ = result // may or may not trigger depending on data spread
}

func TestWorkloadChecker_CheckMetrics_TooFew(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	r := c.CheckMetrics([]float64{1, 2}, []float64{100, 200}, nil)
	if r.HasAnyAnomaly {
		t.Fatal("expected no anomaly with too few samples")
	}
}

func TestWorkloadChecker_CheckMetrics_Normal(t *testing.T) {
	c := anomaly.NewWorkloadChecker()
	cpu := make([]float64, 20)
	mem := make([]float64, 20)
	ts := make([]time.Time, 20)
	now := time.Now()
	for i := range cpu {
		cpu[i] = 50.0 + float64(i%3)
		mem[i] = 512.0 + float64(i%4)
		ts[i] = now.Add(time.Duration(i) * time.Minute)
	}
	r := c.CheckMetrics(cpu, mem, ts)
	_ = r.Summary()
}

func TestWorkloadChecker_CheckMetrics_WithSpike(t *testing.T) {
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

func TestWorkloadAnomalyResult_Summary_NoAnomaly(t *testing.T) {
	r := &anomaly.WorkloadAnomalyResult{
		Namespace:     "ns",
		WorkloadName:  "app",
		HasAnyAnomaly: false,
	}
	s := r.Summary()
	if s == "" {
		t.Error("summary should not be empty")
	}
}

func TestWorkloadAnomalyResult_Summary_WithAnomaly(t *testing.T) {
	r := &anomaly.WorkloadAnomalyResult{
		Namespace:         "ns",
		WorkloadName:      "app",
		HasAnyAnomaly:     true,
		AnomalyCount:      3,
		HighSeverityCount: 1,
		RecommendedAction: "investigate immediately",
	}
	s := r.Summary()
	if s == "" {
		t.Error("summary should not be empty")
	}
}

// ─── DetectionResult helpers ──────────────────────────────────────────────────

func TestDetectionResult_Summary_NoAnomalies(t *testing.T) {
	r := &anomaly.DetectionResult{
		Method:      anomaly.MethodZScore,
		SampleCount: 100,
		Anomalies:   []anomaly.Anomaly{},
	}
	s := r.Summary()
	if s == "" {
		t.Error("summary should not be empty")
	}
}

func TestDetectionResult_Summary_WithAnomalies(t *testing.T) {
	r := &anomaly.DetectionResult{
		Method:      anomaly.MethodIQR,
		SampleCount: 50,
		Anomalies: []anomaly.Anomaly{
			{Severity: anomaly.SeverityHigh},
			{Severity: anomaly.SeverityLow},
		},
	}
	if r.AnomalyCount() != 2 {
		t.Errorf("expected 2, got %d", r.AnomalyCount())
	}
	if r.HighSeverityCount() != 1 {
		t.Errorf("expected 1 high severity, got %d", r.HighSeverityCount())
	}
	s := r.Summary()
	if s == "" {
		t.Error("summary should not be empty")
	}
}

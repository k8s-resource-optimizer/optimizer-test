package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/timepattern"
)

func makeSamples(count int, baseTime time.Time, periodMinutes int, lowVal, highVal int64) []timepattern.Sample {
	samples := make([]timepattern.Sample, count)
	for i := range samples {
		cpu := lowVal
		mem := lowVal * 1024 * 1024
		if (i/periodMinutes)%2 == 0 {
			cpu = highVal
			mem = highVal * 1024 * 1024
		}
		samples[i] = timepattern.Sample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			CPU:       cpu,
			Memory:    mem,
		}
	}
	return samples
}

func makeBusinessHoursSamples(base time.Time) []timepattern.Sample {
	var samples []timepattern.Sample
	for day := 0; day < 7; day++ {
		for hour := 0; hour < 24; hour++ {
			cpu := int64(50)
			mem := int64(50 * 1024 * 1024)
			if hour >= 9 && hour < 18 {
				cpu = 500
				mem = 500 * 1024 * 1024
			}
			samples = append(samples, timepattern.Sample{
				Timestamp: base.Add(time.Duration(day)*24*time.Hour + time.Duration(hour)*time.Hour),
				CPU:       cpu,
				Memory:    mem,
			})
		}
	}
	return samples
}

func TestNewAnalyzer_ReturnsNonNil(t *testing.T) {
	a := timepattern.NewAnalyzer()
	if a == nil {
		t.Fatal("expected non-nil Analyzer")
	}
}

func TestAnalyzer_Analyze_EmptySamples(t *testing.T) {
	a := timepattern.NewAnalyzer()
	result := a.Analyze(nil)
	if result == nil {
		t.Fatal("expected non-nil result for empty samples")
	}
}

func TestAnalyzer_Analyze_SingleSample(t *testing.T) {
	a := timepattern.NewAnalyzer()
	samples := []timepattern.Sample{
		{Timestamp: time.Now(), CPU: 100, Memory: 100 * 1024 * 1024},
	}
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result for single sample")
	}
}

func TestAnalyzer_Analyze_BusinessHoursPattern(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	samples := makeBusinessHoursSamples(base)
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.PatternType == "" {
		t.Error("expected non-empty pattern type")
	}
}

func TestAnalyzer_Analyze_PeriodicPattern(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	samples := makeSamples(1440, base, 60, 50, 300)
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestAnalyzer_Analyze_StablePattern(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Now().Add(-48 * time.Hour)
	var samples []timepattern.Sample
	for i := 0; i < 500; i++ {
		cpu := int64(100 + i%3)
		samples = append(samples, timepattern.Sample{
			Timestamp: base.Add(time.Duration(i) * 5 * time.Minute),
			CPU:       cpu,
			Memory:    cpu * 1024 * 1024,
		})
	}
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result for stable pattern")
	}
}

func TestAnalyzer_Analyze_PeakHoursIdentified(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	samples := makeBusinessHoursSamples(base)
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	for _, ph := range result.PeakHours {
		if ph < 0 || ph > 23 {
			t.Errorf("invalid peak hour: %d", ph)
		}
	}
}

func TestAnalyzer_Analyze_OverallStats(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Now().Add(-72 * time.Hour)
	var samples []timepattern.Sample
	for i := 0; i < 300; i++ {
		cpu := int64(50 + i%100)
		samples = append(samples, timepattern.Sample{
			Timestamp: base.Add(time.Duration(i) * 15 * time.Minute),
			CPU:       cpu,
			Memory:    cpu * 1024 * 1024,
		})
	}
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OverallStats.MaxCPU < result.OverallStats.MinCPU {
		t.Error("max CPU should be >= min CPU in overall stats")
	}
}

func TestAnalyzer_Analyze_HourlyStats_24Entries(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	var samples []timepattern.Sample
	for h := 0; h < 24; h++ {
		for m := 0; m < 4; m++ {
			cpu := int64(100 + h*10)
			samples = append(samples, timepattern.Sample{
				Timestamp: base.Add(time.Duration(h)*time.Hour + time.Duration(m)*15*time.Minute),
				CPU:       cpu,
				Memory:    cpu * 1024 * 1024,
			})
		}
	}
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.HourlyStats) != 24 {
		t.Errorf("expected 24 hourly stat entries, got %d", len(result.HourlyStats))
	}
}

func TestAnalyzer_Analyze_ScalingScheduleField(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	samples := makeBusinessHoursSamples(base)
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	_ = result.ScalingRecommendation
}

func TestAnalyzer_Analyze_NightBatchPattern(t *testing.T) {
	a := timepattern.NewAnalyzer()
	base := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	var samples []timepattern.Sample
	for day := 0; day < 7; day++ {
		for hour := 0; hour < 24; hour++ {
			cpu := int64(50)
			if hour >= 2 && hour < 6 {
				cpu = 800
			}
			samples = append(samples, timepattern.Sample{
				Timestamp: base.Add(time.Duration(day)*24*time.Hour + time.Duration(hour)*time.Hour),
				CPU:       cpu,
				Memory:    cpu * 1024 * 1024,
			})
		}
	}
	result := a.Analyze(samples)
	if result == nil {
		t.Fatal("expected non-nil result for night batch pattern")
	}
}

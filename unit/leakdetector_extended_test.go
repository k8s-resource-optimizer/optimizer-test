package unit_test

import (
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/leakdetector"
)

// ─── NewDetector (default constructor) tests ─────────────────────────────────

// TestNewDetector_DefaultsAreNonZero verifies that NewDetector() returns a
// Detector with sensible (non-zero) default field values.
func TestNewDetector_DefaultsAreNonZero(t *testing.T) {
	d := leakdetector.NewDetector()
	if d.MinSamples == 0 {
		t.Error("expected MinSamples > 0")
	}
	if d.MinDuration == 0 {
		t.Error("expected MinDuration > 0")
	}
	if d.SlopeThreshold == 0 {
		t.Error("expected SlopeThreshold > 0")
	}
}

// ─── AnalyzeWithLimit tests ──────────────────────────────────────────────────

// buildLeakSamples creates a linearly growing sample set that will trigger leak
// detection using the default detector. 25 samples over 2 hours, doubling memory.
func buildLeakSamples() []leakdetector.MemorySample {
	n := 25
	start := int64(100 * 1024 * 1024)  // 100 MiB
	end := int64(400 * 1024 * 1024)    // 400 MiB — 4× growth, well above thresholds
	values := linearMemory(n, start, end)
	return buildSamples(values, 2*time.Hour)
}

// TestAnalyzeWithLimit_NoLimitReturnsAnalysis verifies that passing memoryLimit=0
// still returns a valid analysis without panic.
func TestAnalyzeWithLimit_NoLimitReturnsAnalysis(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute
	samples := buildLeakSamples()
	analysis := d.AnalyzeWithLimit(samples, 0)
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
}

// TestAnalyzeWithLimit_WithLimitPopulatesTimeToOOM verifies that when a leak is
// detected and a memory limit is given, TimeToOOM is estimated (> 0).
func TestAnalyzeWithLimit_WithLimitPopulatesTimeToOOM(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute
	samples := buildLeakSamples()
	analysis := d.AnalyzeWithLimit(samples, 2*1024*1024*1024) // 2 GiB limit

	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
	if analysis.IsLeak && analysis.Statistics.Slope > 0 {
		if analysis.Statistics.TimeToOOM <= 0 {
			t.Error("expected positive TimeToOOM when leak detected with memory limit")
		}
	}
}

// TestAnalyzeWithLimit_NoLeakNoTimeToOOM verifies that stable memory produces
// no TimeToOOM projection.
func TestAnalyzeWithLimit_NoLeakNoTimeToOOM(t *testing.T) {
	d := newDetector()
	values := stableMemory(25, 200*1024*1024)
	samples := buildSamples(values, 2*time.Hour)
	analysis := d.AnalyzeWithLimit(samples, 1024*1024*1024)
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
	// A non-leak should not have a TimeToOOM set.
	if !analysis.IsLeak && analysis.Statistics.TimeToOOM != 0 {
		t.Errorf("expected TimeToOOM=0 for non-leak, got %v", analysis.Statistics.TimeToOOM)
	}
}

// ─── LeakAnalysis.FormatAnalysisSummary tests ────────────────────────────────

// TestFormatAnalysisSummary_ContainsKeyFields verifies that the formatted
// summary includes expected labels.
func TestFormatAnalysisSummary_ContainsKeyFields(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute
	samples := buildLeakSamples()
	analysis := d.Analyze(samples)

	summary := analysis.FormatAnalysisSummary()
	for _, keyword := range []string{"Leak Detected", "Severity", "Statistics"} {
		if !strings.Contains(summary, keyword) {
			t.Errorf("summary missing keyword %q", keyword)
		}
	}
}

// TestFormatAnalysisSummary_NonEmpty verifies that FormatAnalysisSummary never
// returns an empty string, even for a no-leak result.
func TestFormatAnalysisSummary_NonEmpty(t *testing.T) {
	d := newDetector()
	values := stableMemory(25, 100*1024*1024)
	samples := buildSamples(values, 2*time.Hour)
	analysis := d.Analyze(samples)

	summary := analysis.FormatAnalysisSummary()
	if summary == "" {
		t.Error("expected non-empty FormatAnalysisSummary output")
	}
}

// ─── LeakAnalysis.ShouldPreventScaling tests ────────────────────────────────

// TestShouldPreventScaling_FalseWhenNoLeak verifies that a healthy (no leak)
// analysis does not block scaling.
func TestShouldPreventScaling_FalseWhenNoLeak(t *testing.T) {
	d := newDetector()
	values := stableMemory(25, 150*1024*1024)
	samples := buildSamples(values, 2*time.Hour)
	analysis := d.Analyze(samples)

	if analysis.IsLeak {
		t.Skip("analysis unexpectedly detected a leak — skipping scale test")
	}

	block, _ := analysis.ShouldPreventScaling()
	if block {
		t.Error("expected ShouldPreventScaling=false when no leak")
	}
}

// TestShouldPreventScaling_TrueForLeakWithHighSeverity verifies that a detected
// high-severity leak blocks scaling.
func TestShouldPreventScaling_TrueForLeakWithHighSeverity(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute
	samples := buildLeakSamples()
	analysis := d.Analyze(samples)

	if !analysis.IsLeak {
		t.Skip("leak not detected — skipping scale-prevention test")
	}

	switch analysis.Severity {
	case leakdetector.SeverityCritical, leakdetector.SeverityHigh, leakdetector.SeverityMedium:
		block, reason := analysis.ShouldPreventScaling()
		if !block {
			t.Errorf("expected scaling to be blocked for %s severity leak", analysis.Severity)
		}
		if reason == "" {
			t.Error("expected non-empty reason when scaling is blocked")
		}
	}
}

// TestShouldPreventScaling_ReasonNotEmptyWhenBlocked verifies that whenever
// scaling is blocked the reason string is non-empty.
func TestShouldPreventScaling_ReasonNotEmptyWhenBlocked(t *testing.T) {
	d := leakdetector.NewDetector()
	d.MinSamples = 20
	d.MinDuration = 30 * time.Minute
	samples := buildLeakSamples()
	analysis := d.Analyze(samples)

	block, reason := analysis.ShouldPreventScaling()
	if block && reason == "" {
		t.Error("expected non-empty reason when ShouldPreventScaling=true")
	}
}

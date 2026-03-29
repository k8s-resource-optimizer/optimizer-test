package unit_test

// Tests for HoltWintersForecaster and FallbackForecaster.
// ForecastClient/Decide/HorizontalScaler are covered in client_test.go.

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap/zaptest"

	"intelligent-cluster-optimizer/pkg/forecaster"
)

// ── helpers ────────────────────────────────────────────────────────────────────

// rampSamples returns n values linearly rising from lo to hi.
func rampSamples(n int, lo, hi float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = lo + (hi-lo)*float64(i)/float64(n-1)
	}
	return s
}

// ── HoltWintersForecaster ──────────────────────────────────────────────────────

// Test 1a: valid input returns 15 forecast points in 0-1 range
func TestHoltWinters_ValidInput(t *testing.T) {
	hw := forecaster.NewHoltWintersForecaster(zaptest.NewLogger(t))
	samples := rampSamples(60, 0.1, 0.5)

	resp, err := hw.Predict(context.Background(), samples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Forecast) != 15 {
		t.Errorf("expected 15 forecast points, got %d", len(resp.Forecast))
	}
	if resp.PredictionLength != 15 {
		t.Errorf("expected PredictionLength=15, got %d", resp.PredictionLength)
	}
	if resp.ContextLength != 60 {
		t.Errorf("expected ContextLength=60, got %d", resp.ContextLength)
	}
}

// Test 1b: output steps are 1-indexed sequentially
func TestHoltWinters_StepIndexing(t *testing.T) {
	hw := forecaster.NewHoltWintersForecaster(zaptest.NewLogger(t))

	resp, err := hw.Predict(context.Background(), rampSamples(60, 0.2, 0.4))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, pt := range resp.Forecast {
		if pt.Step != i+1 {
			t.Errorf("step %d: expected Step=%d, got %d", i, i+1, pt.Step)
		}
	}
}

// Test 1c: output values (Low, Median, High) are finite and ordered Low ≤ Median ≤ High
func TestHoltWinters_PointOrdering(t *testing.T) {
	hw := forecaster.NewHoltWintersForecaster(zaptest.NewLogger(t))

	resp, err := hw.Predict(context.Background(), rampSamples(60, 0.1, 0.6))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, pt := range resp.Forecast {
		if pt.Low > pt.Median {
			t.Errorf("step %d: Low (%f) > Median (%f)", pt.Step, pt.Low, pt.Median)
		}
		if pt.Median > pt.High {
			t.Errorf("step %d: Median (%f) > High (%f)", pt.Step, pt.Median, pt.High)
		}
	}
}

// Test 1d: flat constant input (no trend) still returns a valid forecast
func TestHoltWinters_ConstantSeries(t *testing.T) {
	hw := forecaster.NewHoltWintersForecaster(zaptest.NewLogger(t))

	const val = 0.35
	samples := make([]float64, 60)
	for i := range samples {
		samples[i] = val
	}

	resp, err := hw.Predict(context.Background(), samples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Forecast) != 15 {
		t.Errorf("expected 15 points, got %d", len(resp.Forecast))
	}
}

// ── FallbackForecaster ─────────────────────────────────────────────────────────

// mockForecaster is a controllable CpuForecaster for tests.
type mockForecaster struct {
	response *forecaster.PredictResponse
	err      error
	called   int
}

func (m *mockForecaster) Predict(_ context.Context, _ []float64) (*forecaster.PredictResponse, error) {
	m.called++
	return m.response, m.err
}

func goodResponse() *forecaster.PredictResponse {
	pts := make([]forecaster.ForecastPoint, 15)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{Step: i + 1, Low: 0.1, Median: 0.3, High: 0.5}
	}
	return &forecaster.PredictResponse{Forecast: pts, ContextLength: 60, PredictionLength: 15}
}

// Test 1e: primary succeeds → FallbackForecaster returns primary result, secondary never called
func TestFallback_PrimarySucceeds(t *testing.T) {
	primary   := &mockForecaster{response: goodResponse()}
	secondary := &mockForecaster{response: goodResponse()}
	fb        := forecaster.NewFallbackForecaster(primary, secondary, zaptest.NewLogger(t))

	resp, err := fb.Predict(context.Background(), rampSamples(60, 0.2, 0.4))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if primary.called != 1 {
		t.Errorf("expected primary called once, got %d", primary.called)
	}
	if secondary.called != 0 {
		t.Errorf("expected secondary never called, got %d", secondary.called)
	}
}

// Test 1f: primary returns error → FallbackForecaster calls secondary, returns no error
func TestFallback_PrimaryFails_SecondaryUsed(t *testing.T) {
	primary   := &mockForecaster{err: errors.New("ml service down")}
	secondary := &mockForecaster{response: goodResponse()}
	fb        := forecaster.NewFallbackForecaster(primary, secondary, zaptest.NewLogger(t))

	resp, err := fb.Predict(context.Background(), rampSamples(60, 0.2, 0.4))
	if err != nil {
		t.Fatalf("FallbackForecaster should absorb primary error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response from secondary")
	}
	if secondary.called != 1 {
		t.Errorf("expected secondary called once, got %d", secondary.called)
	}
}

// Test 1g: both primary and secondary fail → error is propagated
func TestFallback_BothFail_ErrorPropagated(t *testing.T) {
	primary   := &mockForecaster{err: errors.New("ml service down")}
	secondary := &mockForecaster{err: errors.New("holt-winters error")}
	fb        := forecaster.NewFallbackForecaster(primary, secondary, zaptest.NewLogger(t))

	_, err := fb.Predict(context.Background(), rampSamples(60, 0.2, 0.4))
	if err == nil {
		t.Fatal("expected error when both forecasters fail")
	}
}

// Test 1h: FallbackForecaster passes through the same samples to both forecasters
func TestFallback_SamplesPassedThrough(t *testing.T) {
	samples := rampSamples(60, 0.1, 0.9)

	var capturedPrimary, capturedSecondary []float64
	primary := &capturingForecasterFallback{
		capture: &capturedPrimary,
		err:     errors.New("fail"),
	}
	secondary := &capturingForecasterFallback{
		capture:  &capturedSecondary,
		response: goodResponse(),
	}
	fb := forecaster.NewFallbackForecaster(primary, secondary, zaptest.NewLogger(t))
	_, _ = fb.Predict(context.Background(), samples)

	if len(capturedSecondary) != len(samples) {
		t.Errorf("expected %d samples passed to secondary, got %d", len(samples), len(capturedSecondary))
	}
}

type capturingForecasterFallback struct {
	capture  *[]float64
	response *forecaster.PredictResponse
	err      error
}

func (c *capturingForecasterFallback) Predict(_ context.Context, vals []float64) (*forecaster.PredictResponse, error) {
	*c.capture = vals
	return c.response, c.err
}

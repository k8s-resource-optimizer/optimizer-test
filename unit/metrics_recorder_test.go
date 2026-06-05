package unit_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/metrics"
)

func TestPrometheusExporter_RecordReconciliationError(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_err")
	e.RecordReconciliationError("cfg1", "default", "timeout")
	e.RecordReconciliationError("cfg1", "default", "timeout")
}

func TestPrometheusExporter_RecordOptimizationBlocked(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_blocked")
	e.RecordOptimizationBlocked("cfg1", "default", "anomaly")
	e.RecordOptimizationBlocked("cfg1", "default", "circuit_open")
}

func TestPrometheusExporter_RecordRollback(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_rollback")
	e.RecordRollback("cfg1", "default", "oom_detected")
}

func TestPrometheusExporter_RecordPrediction(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_pred")
	e.RecordPrediction("cfg1", "default", "cpu")
	e.RecordPrediction("cfg1", "default", "memory")
}

func TestPrometheusExporter_RecordPeakLoadPredicted(t *testing.T) {
	e := metrics.NewPrometheusExporter("testns_peak")
	e.RecordPeakLoadPredicted("cfg1", "default")
}

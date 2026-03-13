package unit_test

import (
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/trends"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── DeepCopy tests for v1alpha1 types ───────────────────────────────────────

// TestDeepCopy_CircuitBreakerConfig exercises DeepCopy/DeepCopyInto.
func TestDeepCopy_CircuitBreakerConfig(t *testing.T) {
	orig := &v1alpha1.CircuitBreakerConfig{
		Enabled:          true,
		ErrorThreshold:   5,
		SuccessThreshold: 2,
	}
	copy := orig.DeepCopy()
	if copy == nil {
		t.Fatal("expected non-nil DeepCopy")
	}
	if copy.ErrorThreshold != orig.ErrorThreshold {
		t.Errorf("expected ErrorThreshold=%d, got %d", orig.ErrorThreshold, copy.ErrorThreshold)
	}
	// nil receiver
	var nilCBC *v1alpha1.CircuitBreakerConfig
	if nilCBC.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil receiver")
	}
}

// TestDeepCopy_HPAAwareness exercises DeepCopy for HPAAwareness.
func TestDeepCopy_HPAAwareness(t *testing.T) {
	orig := &v1alpha1.HPAAwareness{
		Enabled:        true,
		ConflictPolicy: v1alpha1.HPAConflictPolicySkip,
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.ConflictPolicy != orig.ConflictPolicy {
		t.Errorf("HPAAwareness DeepCopy mismatch: got %+v", copy)
	}
	var nilObj *v1alpha1.HPAAwareness
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil HPAAwareness")
	}
}

// TestDeepCopy_MaintenanceWindow exercises DeepCopy for MaintenanceWindow.
func TestDeepCopy_MaintenanceWindow(t *testing.T) {
	orig := &v1alpha1.MaintenanceWindow{
		Schedule: "0 0 * * 6",
		Duration: "2h",
		Timezone: "UTC",
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.Schedule != orig.Schedule {
		t.Errorf("MaintenanceWindow DeepCopy mismatch: got %+v", copy)
	}
	var nilObj *v1alpha1.MaintenanceWindow
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil MaintenanceWindow")
	}
}

// TestDeepCopy_OptimizerCondition exercises DeepCopy for OptimizerCondition.
func TestDeepCopy_OptimizerCondition(t *testing.T) {
	now := metav1.NewTime(time.Now())
	orig := &v1alpha1.OptimizerCondition{
		Type:               "Ready",
		Status:             "True",
		Reason:             "AllOK",
		Message:            "system healthy",
		LastTransitionTime: now,
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.Type != orig.Type {
		t.Errorf("OptimizerCondition DeepCopy mismatch: got %+v", copy)
	}
	var nilObj *v1alpha1.OptimizerCondition
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerCondition")
	}
}

// TestDeepCopy_OptimizerConfig exercises DeepCopy/DeepCopyObject for OptimizerConfig.
func TestDeepCopy_OptimizerConfig(t *testing.T) {
	orig := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default", "production"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:    95,
				MemoryPercentile: 95,
			},
			ResourceThresholds: &v1alpha1.ResourceThresholds{
				CPU:    &v1alpha1.ResourceLimit{Min: "100m", Max: "2000m"},
				Memory: &v1alpha1.ResourceLimit{Min: "128Mi", Max: "4Gi"},
			},
		},
	}
	copy := orig.DeepCopy()
	if copy == nil {
		t.Fatal("expected non-nil DeepCopy of OptimizerConfig")
	}
	if len(copy.Spec.TargetNamespaces) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(copy.Spec.TargetNamespaces))
	}
	if copy.Spec.ResourceThresholds == nil || copy.Spec.ResourceThresholds.CPU == nil {
		t.Error("expected ResourceThresholds.CPU to be copied")
	}

	// DeepCopyObject
	obj := orig.DeepCopyObject()
	if obj == nil {
		t.Error("expected non-nil DeepCopyObject")
	}

	// nil receiver
	var nilObj *v1alpha1.OptimizerConfig
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil OptimizerConfig")
	}
}

// TestDeepCopy_OptimizerConfigList exercises DeepCopy/DeepCopyObject for OptimizerConfigList.
func TestDeepCopy_OptimizerConfigList(t *testing.T) {
	orig := &v1alpha1.OptimizerConfigList{
		Items: []v1alpha1.OptimizerConfig{
			{
				Spec: v1alpha1.OptimizerConfigSpec{
					Enabled:          true,
					TargetNamespaces: []string{"ns1"},
				},
			},
		},
	}
	copy := orig.DeepCopy()
	if copy == nil || len(copy.Items) != 1 {
		t.Errorf("OptimizerConfigList DeepCopy mismatch: got %+v", copy)
	}
	obj := orig.DeepCopyObject()
	if obj == nil {
		t.Error("expected non-nil DeepCopyObject for list")
	}
	var nilObj *v1alpha1.OptimizerConfigList
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil list")
	}
}

// TestDeepCopy_OptimizerConfigSpec exercises DeepCopyInto branch for optional fields.
func TestDeepCopy_OptimizerConfigSpec(t *testing.T) {
	rolling := v1alpha1.RollingUpdateConfig{MaxUnavailable: "1", MaxSurge: "1"}
	orig := &v1alpha1.OptimizerConfigSpec{
		Enabled:          true,
		TargetNamespaces: []string{"dev"},
		Strategy:         v1alpha1.StrategyAggressive,
		MaintenanceWindows: []v1alpha1.MaintenanceWindow{
			{Schedule: "0 2 * * *", Duration: "1h"},
		},
		UpdateStrategy: &v1alpha1.UpdateStrategy{
			Type:          "RollingUpdate",
			RollingUpdate: &rolling,
		},
		HPAAwareness: &v1alpha1.HPAAwareness{Enabled: true, ConflictPolicy: v1alpha1.HPAConflictPolicyWarn},
		PDBAwareness: &v1alpha1.PDBAwareness{Enabled: true, RespectMinAvailable: true},
		CircuitBreaker: &v1alpha1.CircuitBreakerConfig{Enabled: true},
		ExcludeWorkloads: []string{"my-job"},
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.UpdateStrategy == nil || copy.UpdateStrategy.RollingUpdate == nil {
		t.Fatal("expected full DeepCopy of OptimizerConfigSpec")
	}
	if copy.HPAAwareness == nil || copy.PDBAwareness == nil || copy.CircuitBreaker == nil {
		t.Error("expected optional fields to be deep-copied")
	}
	if len(copy.ExcludeWorkloads) != 1 {
		t.Errorf("expected 1 ExcludeWorkload, got %d", len(copy.ExcludeWorkloads))
	}

	var nilObj *v1alpha1.OptimizerConfigSpec
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerConfigSpec")
	}
}

// TestDeepCopy_OptimizerConfigStatus exercises DeepCopy with timestamps and conditions.
func TestDeepCopy_OptimizerConfigStatus(t *testing.T) {
	now := metav1.NewTime(time.Now())
	orig := &v1alpha1.OptimizerConfigStatus{
		Phase:                    "Running",
		LastRecommendationTime:   &now,
		LastUpdateTime:           &now,
		NextMaintenanceWindow:    &now,
		Conditions: []v1alpha1.OptimizerCondition{
			{Type: "Ready", Status: "True", LastTransitionTime: now},
		},
	}
	copy := orig.DeepCopy()
	if copy == nil {
		t.Fatal("expected non-nil DeepCopy of OptimizerConfigStatus")
	}
	if copy.LastRecommendationTime == nil {
		t.Error("expected LastRecommendationTime to be copied")
	}
	if len(copy.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(copy.Conditions))
	}
	var nilObj *v1alpha1.OptimizerConfigStatus
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerConfigStatus")
	}
}

// TestDeepCopy_PDBAwareness exercises DeepCopy for PDBAwareness.
func TestDeepCopy_PDBAwareness(t *testing.T) {
	orig := &v1alpha1.PDBAwareness{Enabled: true, RespectMinAvailable: true}
	copy := orig.DeepCopy()
	if copy == nil || copy.RespectMinAvailable != orig.RespectMinAvailable {
		t.Errorf("PDBAwareness DeepCopy mismatch")
	}
	var nilObj *v1alpha1.PDBAwareness
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil PDBAwareness")
	}
}

// TestDeepCopy_RecommendationConfig exercises DeepCopy for RecommendationConfig.
func TestDeepCopy_RecommendationConfig(t *testing.T) {
	orig := &v1alpha1.RecommendationConfig{
		CPUPercentile:    90,
		MemoryPercentile: 95,
		SafetyMargin:     0.1,
		MinSamples:       20,
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.CPUPercentile != orig.CPUPercentile {
		t.Errorf("RecommendationConfig DeepCopy mismatch")
	}
	var nilObj *v1alpha1.RecommendationConfig
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil RecommendationConfig")
	}
}

// TestDeepCopy_ResourceLimit exercises DeepCopy for ResourceLimit.
func TestDeepCopy_ResourceLimit(t *testing.T) {
	orig := &v1alpha1.ResourceLimit{Min: "100m", Max: "2"}
	copy := orig.DeepCopy()
	if copy == nil || copy.Min != orig.Min {
		t.Errorf("ResourceLimit DeepCopy mismatch")
	}
	var nilObj *v1alpha1.ResourceLimit
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil ResourceLimit")
	}
}

// TestDeepCopy_ResourceThresholds exercises DeepCopy with CPU and Memory set.
func TestDeepCopy_ResourceThresholds(t *testing.T) {
	orig := &v1alpha1.ResourceThresholds{
		CPU:    &v1alpha1.ResourceLimit{Min: "50m", Max: "500m"},
		Memory: &v1alpha1.ResourceLimit{Min: "64Mi", Max: "2Gi"},
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.CPU == nil || copy.Memory == nil {
		t.Fatal("expected full ResourceThresholds DeepCopy")
	}
	if copy.CPU.Min != orig.CPU.Min || copy.Memory.Max != orig.Memory.Max {
		t.Error("ResourceThresholds values not copied correctly")
	}
	var nilObj *v1alpha1.ResourceThresholds
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil ResourceThresholds")
	}
}

// TestDeepCopy_RollingUpdateConfig exercises DeepCopy for RollingUpdateConfig.
func TestDeepCopy_RollingUpdateConfig(t *testing.T) {
	orig := &v1alpha1.RollingUpdateConfig{MaxUnavailable: "2", MaxSurge: "1"}
	copy := orig.DeepCopy()
	if copy == nil || copy.MaxUnavailable != "2" {
		t.Errorf("RollingUpdateConfig DeepCopy mismatch")
	}
	var nilObj *v1alpha1.RollingUpdateConfig
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil RollingUpdateConfig")
	}
}

// TestDeepCopy_UpdateStrategy exercises DeepCopy with and without RollingUpdate.
func TestDeepCopy_UpdateStrategy(t *testing.T) {
	rolling := v1alpha1.RollingUpdateConfig{MaxUnavailable: "1", MaxSurge: "0"}
	orig := &v1alpha1.UpdateStrategy{
		Type:          "RollingUpdate",
		RollingUpdate: &rolling,
	}
	copy := orig.DeepCopy()
	if copy == nil || copy.RollingUpdate == nil {
		t.Fatal("expected full UpdateStrategy DeepCopy with RollingUpdate")
	}
	// Without RollingUpdate
	orig2 := &v1alpha1.UpdateStrategy{Type: "Recreate"}
	copy2 := orig2.DeepCopy()
	if copy2 == nil || copy2.RollingUpdate != nil {
		t.Error("expected nil RollingUpdate for Recreate strategy")
	}
	var nilObj *v1alpha1.UpdateStrategy
	if nilObj.DeepCopy() != nil {
		t.Error("expected nil for nil UpdateStrategy")
	}
}

// ─── trends growth helper functions ─────────────────────────────────────────

// TestCalculateGrowthRates_BasicCall exercises the growth rate calculation.
func TestCalculateGrowthRates_Basic(t *testing.T) {
	data := make([]float64, 10)
	for i := range data {
		data[i] = float64(100 + i*10)
	}
	ts := make([]time.Time, 10)
	base := time.Now().Add(-10 * time.Hour)
	for i := range ts {
		ts[i] = base.Add(time.Duration(i) * time.Hour)
	}
	rate := trends.CalculateGrowthRates(data, ts)
	_ = rate // just exercise the function
}

// TestCalculateCAGR_Basic exercises the CAGR calculation.
func TestCalculateCAGR_Basic(t *testing.T) {
	data := []float64{100, 110, 121, 133, 146}
	cagr := trends.CalculateCAGR(data, 4)
	if cagr < 0 {
		t.Errorf("expected positive CAGR for growing data, got %.4f", cagr)
	}
}

// TestCalculateAcceleration_Basic exercises the acceleration calculation.
func TestCalculateAcceleration_Basic(t *testing.T) {
	data := []float64{1, 2, 4, 8, 16, 32, 64}
	acc := trends.CalculateAcceleration(data)
	_ = acc // just exercise the function
}

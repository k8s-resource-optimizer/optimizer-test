package integration_test

import (
	"testing"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDeepCopy_AllTypes exercises all DeepCopy/DeepCopyInto methods in v1alpha1.
func TestDeepCopy_AllTypes(t *testing.T) {
	// CircuitBreakerConfig
	cbc := &v1alpha1.CircuitBreakerConfig{
		Enabled:          true,
		ErrorThreshold:   3,
		SuccessThreshold: 2,
		Timeout:          "5m",
	}
	cbc2 := cbc.DeepCopy()
	if cbc2 == nil || cbc2.ErrorThreshold != 3 {
		t.Error("CircuitBreakerConfig DeepCopy failed")
	}
	var cbcNil *v1alpha1.CircuitBreakerConfig
	if cbcNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil CircuitBreakerConfig")
	}

	// HPAAwareness
	hpa := &v1alpha1.HPAAwareness{
		Enabled:        true,
		ConflictPolicy: "SkipCPU",
	}
	hpa2 := hpa.DeepCopy()
	if hpa2 == nil || hpa2.ConflictPolicy != "SkipCPU" {
		t.Error("HPAAwareness DeepCopy failed")
	}
	var hpaNil *v1alpha1.HPAAwareness
	if hpaNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil HPAAwareness")
	}

	// MaintenanceWindow
	mw := &v1alpha1.MaintenanceWindow{
		Schedule: "0 2 * * 0",
		Duration: "2h",
		Timezone: "UTC",
	}
	mw2 := mw.DeepCopy()
	if mw2 == nil || mw2.Schedule != "0 2 * * 0" {
		t.Error("MaintenanceWindow DeepCopy failed")
	}
	var mwNil *v1alpha1.MaintenanceWindow
	if mwNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil MaintenanceWindow")
	}

	// OptimizerCondition
	cond := &v1alpha1.OptimizerCondition{
		Type:    "Ready",
		Status:  "True",
		Message: "all good",
	}
	cond2 := cond.DeepCopy()
	if cond2 == nil || cond2.Type != "Ready" {
		t.Error("OptimizerCondition DeepCopy failed")
	}
	var condNil *v1alpha1.OptimizerCondition
	if condNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil OptimizerCondition")
	}

	// PDBAwareness
	pda := &v1alpha1.PDBAwareness{
		Enabled:            true,
		RespectMinAvailable: true,
	}
	pda2 := pda.DeepCopy()
	if pda2 == nil || !pda2.RespectMinAvailable {
		t.Error("PDBAwareness DeepCopy failed")
	}
	var pdaNil *v1alpha1.PDBAwareness
	if pdaNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil PDBAwareness")
	}

	// RecommendationConfig
	rc := &v1alpha1.RecommendationConfig{
		CPUPercentile:   95,
		MinSamples:      10,
		SafetyMargin:    1.1,
		HistoryDuration: "24h",
	}
	rc2 := rc.DeepCopy()
	if rc2 == nil || rc2.CPUPercentile != 95 {
		t.Error("RecommendationConfig DeepCopy failed")
	}
	var rcNil *v1alpha1.RecommendationConfig
	if rcNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil RecommendationConfig")
	}

	// ResourceLimit
	rl := &v1alpha1.ResourceLimit{
		Min: "100m",
		Max: "2",
	}
	rl2 := rl.DeepCopy()
	if rl2 == nil || rl2.Min != "100m" {
		t.Error("ResourceLimit DeepCopy failed")
	}
	var rlNil *v1alpha1.ResourceLimit
	if rlNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil ResourceLimit")
	}

	// ResourceThresholds
	rt := &v1alpha1.ResourceThresholds{
		CPU:    &v1alpha1.ResourceLimit{Min: "10m", Max: "4"},
		Memory: &v1alpha1.ResourceLimit{Min: "64Mi", Max: "8Gi"},
	}
	rt2 := rt.DeepCopy()
	if rt2 == nil || rt2.CPU == nil {
		t.Error("ResourceThresholds DeepCopy failed")
	}
	var rtNil *v1alpha1.ResourceThresholds
	if rtNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil ResourceThresholds")
	}

	// RollingUpdateConfig
	ruc := &v1alpha1.RollingUpdateConfig{
		MaxUnavailable: "25%",
		MaxSurge:       "25%",
	}
	ruc2 := ruc.DeepCopy()
	if ruc2 == nil || ruc2.MaxUnavailable != "25%" {
		t.Error("RollingUpdateConfig DeepCopy failed")
	}
	var rucNil *v1alpha1.RollingUpdateConfig
	if rucNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil RollingUpdateConfig")
	}

	// UpdateStrategy
	us := &v1alpha1.UpdateStrategy{
		Type: v1alpha1.UpdateStrategyInPlace,
	}
	us2 := us.DeepCopy()
	if us2 == nil || us2.Type != v1alpha1.UpdateStrategyInPlace {
		t.Error("UpdateStrategy DeepCopy failed")
	}
	var usNil *v1alpha1.UpdateStrategy
	if usNil.DeepCopy() != nil {
		t.Error("expected nil DeepCopy for nil UpdateStrategy")
	}
}

// TestDeepCopy_OptimizerConfig exercises OptimizerConfig DeepCopy with full spec.
func TestDeepCopy_OptimizerConfig(t *testing.T) {
	cfg := &v1alpha1.OptimizerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "default",
		},
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default", "production"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   95,
				MinSamples:      10,
				SafetyMargin:    1.1,
				HistoryDuration: "24h",
			},
			CircuitBreaker: &v1alpha1.CircuitBreakerConfig{
				Enabled:          true,
				ErrorThreshold:   3,
				SuccessThreshold: 2,
				Timeout:          "5m",
			},
			HPAAwareness: &v1alpha1.HPAAwareness{
				Enabled:        true,
				ConflictPolicy: "SkipCPU",
			},
			PDBAwareness: &v1alpha1.PDBAwareness{
				Enabled:            true,
				RespectMinAvailable: true,
			},
		},
	}

	cfg2 := cfg.DeepCopy()
	if cfg2 == nil {
		t.Fatal("OptimizerConfig DeepCopy returned nil")
	}
	if cfg2.Name != "test-config" {
		t.Errorf("expected name 'test-config', got %s", cfg2.Name)
	}
	if cfg2.Spec.Recommendations == nil {
		t.Error("expected non-nil Recommendations after DeepCopy")
	}
	if len(cfg2.Spec.TargetNamespaces) != 2 {
		t.Errorf("expected 2 target namespaces, got %d", len(cfg2.Spec.TargetNamespaces))
	}

	// Verify independence
	cfg2.Spec.TargetNamespaces[0] = "modified"
	if cfg.Spec.TargetNamespaces[0] == "modified" {
		t.Error("DeepCopy should be independent of original")
	}

	// DeepCopyObject
	obj := cfg.DeepCopyObject()
	if obj == nil {
		t.Error("DeepCopyObject returned nil")
	}

	// Nil case
	var nilCfg *v1alpha1.OptimizerConfig
	if nilCfg.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerConfig DeepCopy")
	}
}

// TestDeepCopy_OptimizerConfigList exercises OptimizerConfigList DeepCopy.
func TestDeepCopy_OptimizerConfigList(t *testing.T) {
	list := &v1alpha1.OptimizerConfigList{
		Items: []v1alpha1.OptimizerConfig{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "config-1"},
				Spec: v1alpha1.OptimizerConfigSpec{
					Enabled:  true,
					Strategy: v1alpha1.StrategyBalanced,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "config-2"},
				Spec: v1alpha1.OptimizerConfigSpec{
					Enabled:  false,
					Strategy: v1alpha1.StrategyConservative,
				},
			},
		},
	}

	list2 := list.DeepCopy()
	if list2 == nil {
		t.Fatal("OptimizerConfigList DeepCopy returned nil")
	}
	if len(list2.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(list2.Items))
	}

	// DeepCopyObject
	obj := list.DeepCopyObject()
	if obj == nil {
		t.Error("OptimizerConfigList DeepCopyObject returned nil")
	}

	// Nil case
	var nilList *v1alpha1.OptimizerConfigList
	if nilList.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerConfigList DeepCopy")
	}
}

// TestDeepCopy_OptimizerConfigStatus exercises OptimizerConfigStatus DeepCopy.
func TestDeepCopy_OptimizerConfigStatus(t *testing.T) {
	status := &v1alpha1.OptimizerConfigStatus{
		ObservedGeneration: 5,
		Conditions: []v1alpha1.OptimizerCondition{
			{Type: "Ready", Status: "True", Message: "running"},
		},
	}

	status2 := status.DeepCopy()
	if status2 == nil {
		t.Fatal("OptimizerConfigStatus DeepCopy returned nil")
	}
	if len(status2.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(status2.Conditions))
	}

	var nilStatus *v1alpha1.OptimizerConfigStatus
	if nilStatus.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerConfigStatus DeepCopy")
	}
}

// TestDeepCopy_OptimizerConfigSpec exercises OptimizerConfigSpec DeepCopy with all fields.
func TestDeepCopy_OptimizerConfigSpec(t *testing.T) {
	spec := &v1alpha1.OptimizerConfigSpec{
		Enabled:          true,
		TargetNamespaces: []string{"ns1", "ns2"},
		Strategy:         v1alpha1.StrategyBalanced,
		Recommendations: &v1alpha1.RecommendationConfig{
			CPUPercentile: 90,
			MinSamples:    5,
		},
		CircuitBreaker: &v1alpha1.CircuitBreakerConfig{
			Enabled:        true,
			ErrorThreshold: 5,
		},
		HPAAwareness: &v1alpha1.HPAAwareness{
			Enabled: true,
		},
		PDBAwareness: &v1alpha1.PDBAwareness{
			Enabled: true,
		},
		UpdateStrategy: &v1alpha1.UpdateStrategy{
			Type: v1alpha1.UpdateStrategyRollingUpdate,
		},
		ResourceThresholds: &v1alpha1.ResourceThresholds{
			CPU: &v1alpha1.ResourceLimit{Min: "10m", Max: "4"},
		},
	}

	spec2 := spec.DeepCopy()
	if spec2 == nil {
		t.Fatal("OptimizerConfigSpec DeepCopy returned nil")
	}
	if spec2.Recommendations == nil || spec2.CircuitBreaker == nil {
		t.Error("expected all sub-fields to be non-nil after DeepCopy")
	}

	var nilSpec *v1alpha1.OptimizerConfigSpec
	if nilSpec.DeepCopy() != nil {
		t.Error("expected nil for nil OptimizerConfigSpec DeepCopy")
	}
}

// TestDeepCopy_UpdateStrategyWithRollingUpdate exercises the RollingUpdate pointer branch.
func TestDeepCopy_UpdateStrategyWithRollingUpdate(t *testing.T) {
	us := &v1alpha1.UpdateStrategy{
		Type: v1alpha1.UpdateStrategyRollingUpdate,
		RollingUpdate: &v1alpha1.RollingUpdateConfig{
			MaxUnavailable: "25%",
			MaxSurge:       "25%",
		},
	}
	us2 := us.DeepCopy()
	if us2 == nil || us2.RollingUpdate == nil {
		t.Fatal("expected non-nil RollingUpdate after DeepCopy")
	}
	if us2.RollingUpdate.MaxUnavailable != "25%" {
		t.Errorf("expected MaxUnavailable '25%%', got %s", us2.RollingUpdate.MaxUnavailable)
	}
	// Verify independence
	us2.RollingUpdate.MaxUnavailable = "50%"
	if us.RollingUpdate.MaxUnavailable == "50%" {
		t.Error("DeepCopy should be independent")
	}
}

// TestDeepCopy_OptimizerConfigSpecWithAllFields exercises all DeepCopyInto branches.
func TestDeepCopy_OptimizerConfigSpecWithAllFields(t *testing.T) {
	spec := &v1alpha1.OptimizerConfigSpec{
		Enabled:          true,
		TargetNamespaces: []string{"ns1", "ns2"},
		Strategy:         v1alpha1.StrategyBalanced,
		MaintenanceWindows: []v1alpha1.MaintenanceWindow{
			{Schedule: "0 2 * * 0", Duration: "2h", Timezone: "UTC"},
		},
		TargetResources: []v1alpha1.TargetResourceType{
			v1alpha1.TargetResourceDeployments,
			v1alpha1.TargetResourceStatefulSets,
		},
		ExcludeWorkloads: []string{"kube-system/*"},
		UpdateStrategy: &v1alpha1.UpdateStrategy{
			Type: v1alpha1.UpdateStrategyRollingUpdate,
			RollingUpdate: &v1alpha1.RollingUpdateConfig{
				MaxUnavailable: "1",
				MaxSurge:       "1",
			},
		},
	}

	spec2 := spec.DeepCopy()
	if spec2 == nil {
		t.Fatal("spec DeepCopy returned nil")
	}
	if len(spec2.MaintenanceWindows) != 1 {
		t.Errorf("expected 1 maintenance window, got %d", len(spec2.MaintenanceWindows))
	}
	if len(spec2.TargetResources) != 2 {
		t.Errorf("expected 2 target resources, got %d", len(spec2.TargetResources))
	}
	if spec2.UpdateStrategy == nil || spec2.UpdateStrategy.RollingUpdate == nil {
		t.Error("expected non-nil UpdateStrategy.RollingUpdate after DeepCopy")
	}
}

// TestDeepCopy_OptimizerConfigStatusWithTimes exercises time pointer branches in DeepCopyInto.
func TestDeepCopy_OptimizerConfigStatusWithTimes(t *testing.T) {
	now := metav1.Now()
	status := &v1alpha1.OptimizerConfigStatus{
		ObservedGeneration:     3,
		LastRecommendationTime: &now,
		LastUpdateTime:         &now,
		NextMaintenanceWindow:  &now,
		Conditions: []v1alpha1.OptimizerCondition{
			{Type: "Ready", Status: "True"},
		},
	}

	status2 := status.DeepCopy()
	if status2 == nil {
		t.Fatal("expected non-nil status after DeepCopy")
	}
	if status2.LastRecommendationTime == nil {
		t.Error("expected non-nil LastRecommendationTime")
	}
	if status2.LastUpdateTime == nil {
		t.Error("expected non-nil LastUpdateTime")
	}
	if status2.NextMaintenanceWindow == nil {
		t.Error("expected non-nil NextMaintenanceWindow")
	}
}

package integration_test

import (
	"testing"
	"time"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/profile"
)

// ─── ProfileManager ───────────────────────────────────────────────────────────

func TestIntProfile_DefaultProfiles(t *testing.T) {
	pm := profile.NewProfileManager()
	envs := []profile.EnvironmentType{
		profile.EnvironmentProduction,
		profile.EnvironmentStaging,
		profile.EnvironmentDevelopment,
		profile.EnvironmentTest,
	}
	for _, env := range envs {
		p, err := pm.GetProfileByEnvironment(env)
		if err != nil {
			t.Errorf("missing default profile for %s: %v", env, err)
			continue
		}
		if p.Name == "" {
			t.Errorf("profile name empty for %s", env)
		}
	}
}

func TestIntProfile_GetByName(t *testing.T) {
	pm := profile.NewProfileManager()
	names := []string{"production", "staging", "development", "test"}
	for _, name := range names {
		p, err := pm.GetProfile(name)
		if err != nil {
			t.Errorf("GetProfile(%s) failed: %v", name, err)
			continue
		}
		if p.Name != name {
			t.Errorf("expected name=%s, got %s", name, p.Name)
		}
	}
}

func TestIntProfile_GetByName_NotFound(t *testing.T) {
	pm := profile.NewProfileManager()
	_, err := pm.GetProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestIntProfile_GetByEnvironment_NotFound(t *testing.T) {
	pm := profile.NewProfileManager()
	_, err := pm.GetProfileByEnvironment("unknown-env")
	if err == nil {
		t.Fatal("expected error for nonexistent environment")
	}
}

func TestIntProfile_ListProfiles(t *testing.T) {
	pm := profile.NewProfileManager()
	list := pm.ListProfiles()
	if len(list) < 4 {
		t.Fatalf("expected at least 4 profiles, got %d", len(list))
	}
}

func TestIntProfile_RegisterCustom(t *testing.T) {
	pm := profile.NewProfileManager()
	custom := profile.CustomProfile("my-profile", "Custom profile", profile.ProfileSettings{
		Strategy:         "balanced",
		CPUPercentile:    90,
		MemoryPercentile: 90,
		SafetyMargin:     1.2,
		MinSamples:       50,
		MinConfidence:    60.0,
	})
	pm.RegisterProfile(custom)

	p, err := pm.GetProfile("my-profile")
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if p.Environment != profile.EnvironmentCustom {
		t.Errorf("expected custom environment, got %s", p.Environment)
	}
}

// ─── Profile builders ─────────────────────────────────────────────────────────

func TestIntProfile_ProductionSettings(t *testing.T) {
	p := profile.ProductionProfile()
	if p.Settings.SafetyMargin < 1.3 {
		t.Error("production safety margin should be high (conservative)")
	}
	if !p.Settings.RequireApproval {
		t.Error("production should require approval")
	}
	if !p.Settings.DryRunByDefault {
		t.Error("production should dry-run by default")
	}
	if p.Settings.CPUPercentile < 95 {
		t.Errorf("production CPU percentile should be high, got %d", p.Settings.CPUPercentile)
	}
}

func TestIntProfile_StagingSettings(t *testing.T) {
	p := profile.StagingProfile()
	if p.Settings.RequireApproval {
		t.Error("staging should not require approval")
	}
	if p.Settings.DryRunByDefault {
		t.Error("staging should not dry-run by default")
	}
}

func TestIntProfile_DevelopmentSettings(t *testing.T) {
	p := profile.DevelopmentProfile()
	if p.Settings.Strategy != "aggressive" {
		t.Errorf("development should be aggressive, got %s", p.Settings.Strategy)
	}
	if p.Settings.MaxChangePercent < 40 {
		t.Errorf("development max change should be large, got %.1f", p.Settings.MaxChangePercent)
	}
}

func TestIntProfile_TestSettings(t *testing.T) {
	p := profile.TestProfile()
	if p.Settings.CircuitBreakerEnabled {
		t.Error("test profile should not have circuit breaker")
	}
	if p.Settings.RollbackOnError {
		t.Error("test profile should not rollback on error")
	}
}

// ─── ProfileSettings.Merge ────────────────────────────────────────────────────

func TestIntProfile_Merge_NilOverrides(t *testing.T) {
	base := profile.ProductionProfile().Settings
	result := base.Merge(nil)
	if result.Strategy != base.Strategy {
		t.Error("nil override should not change strategy")
	}
	if result.SafetyMargin != base.SafetyMargin {
		t.Error("nil override should not change safety margin")
	}
}

func TestIntProfile_Merge_OverridesStrategy(t *testing.T) {
	base := profile.ProductionProfile().Settings
	overrides := &profile.ProfileSettings{
		Strategy:         "aggressive",
		CPUPercentile:    80,
		MemoryPercentile: 80,
		SafetyMargin:     1.05,
		MinSamples:       20,
		MinConfidence:    20.0,
	}
	result := base.Merge(overrides)
	if result.Strategy != "aggressive" {
		t.Errorf("expected aggressive, got %s", result.Strategy)
	}
	if result.CPUPercentile != 80 {
		t.Errorf("expected 80, got %d", result.CPUPercentile)
	}
}

func TestIntProfile_Merge_ZeroValuesNotOverridden(t *testing.T) {
	base := profile.ProductionProfile().Settings
	overrides := &profile.ProfileSettings{} // all zero values
	result := base.Merge(overrides)
	if result.Strategy != base.Strategy {
		t.Error("zero override should not change strategy")
	}
	if result.MinSamples != base.MinSamples {
		t.Errorf("zero MinSamples should not change value: got %d", result.MinSamples)
	}
}

func TestIntProfile_Merge_AllFields(t *testing.T) {
	base := profile.DevelopmentProfile().Settings
	overrides := &profile.ProfileSettings{
		Strategy:                "conservative",
		CPUPercentile:           99,
		MemoryPercentile:        99,
		SafetyMargin:            1.5,
		MinSamples:              200,
		HistoryDuration:         7 * 24 * time.Hour,
		MinConfidence:           80.0,
		ApplyDelay:              24 * time.Hour,
		MaxChangePercent:        10.0,
		CircuitBreakerThreshold: 2,
		MinCPUMillicores:        100,
		MinMemoryMegabytes:      128,
		MaxCPUMillicores:        8000,
		MaxMemoryMegabytes:      32 * 1024,
	}
	result := base.Merge(overrides)
	if result.Strategy != "conservative" {
		t.Errorf("expected conservative, got %s", result.Strategy)
	}
	if result.SafetyMargin != 1.5 {
		t.Errorf("expected 1.5, got %f", result.SafetyMargin)
	}
}

// ─── ProfileSettings.Validate ─────────────────────────────────────────────────

func TestIntProfile_Validate_Production(t *testing.T) {
	s := profile.ProductionProfile().Settings
	if err := s.Validate(); err != nil {
		t.Fatalf("production profile should be valid: %v", err)
	}
}

func TestIntProfile_Validate_Staging(t *testing.T) {
	s := profile.StagingProfile().Settings
	if err := s.Validate(); err != nil {
		t.Fatalf("staging profile should be valid: %v", err)
	}
}

func TestIntProfile_Validate_Development(t *testing.T) {
	s := profile.DevelopmentProfile().Settings
	if err := s.Validate(); err != nil {
		t.Fatalf("development profile should be valid: %v", err)
	}
}

func TestIntProfile_Validate_InvalidCPUPercentile(t *testing.T) {
	s := profile.ProductionProfile().Settings
	s.CPUPercentile = 30
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for low CPU percentile")
	}
}

func TestIntProfile_Validate_HighCPUPercentile(t *testing.T) {
	s := profile.ProductionProfile().Settings
	s.CPUPercentile = 100
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for CPU percentile > 99")
	}
}

func TestIntProfile_Validate_InvalidMemoryPercentile(t *testing.T) {
	s := profile.StagingProfile().Settings
	s.MemoryPercentile = 10
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for low memory percentile")
	}
}

func TestIntProfile_Validate_InvalidSafetyMargin_Low(t *testing.T) {
	s := profile.DevelopmentProfile().Settings
	s.SafetyMargin = 0.5
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for safety margin < 1")
	}
}

func TestIntProfile_Validate_InvalidSafetyMargin_High(t *testing.T) {
	s := profile.DevelopmentProfile().Settings
	s.SafetyMargin = 5.0
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for safety margin > 3")
	}
}

func TestIntProfile_Validate_ZeroMinSamples(t *testing.T) {
	s := profile.TestProfile().Settings
	s.MinSamples = 0
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for MinSamples=0")
	}
}

func TestIntProfile_Validate_InvalidConfidence(t *testing.T) {
	s := profile.StagingProfile().Settings
	s.MinConfidence = 110.0
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for confidence > 100")
	}
}

func TestIntProfile_Validate_NegativeMaxChange(t *testing.T) {
	s := profile.TestProfile().Settings
	s.MaxChangePercent = -5.0
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for negative MaxChangePercent")
	}
}

func TestIntProfile_Validate_InvalidStrategy(t *testing.T) {
	s := profile.StagingProfile().Settings
	s.Strategy = "unknown"
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

// ─── Profile.String / Summary ─────────────────────────────────────────────────

func TestIntProfile_String(t *testing.T) {
	p := profile.ProductionProfile()
	s := p.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestIntProfile_Summary(t *testing.T) {
	p := profile.StagingProfile()
	s := p.Summary()
	if s == "" {
		t.Error("Summary() should not be empty")
	}
}

// ─── Resolver ─────────────────────────────────────────────────────────────────

func TestIntResolver_NoProfile_DefaultsToBalanced(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Enabled: true,
			DryRun:  false,
		},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved.Strategy == "" {
		t.Error("expected non-empty strategy")
	}
	s := resolved.Summary()
	if s == "" {
		t.Error("Summary empty")
	}
}

func TestIntResolver_ProductionProfile(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "production",
			DryRun:  false,
		},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve with production profile failed: %v", err)
	}
	if resolved.ProfileName != "production" {
		t.Errorf("expected production, got %s", resolved.ProfileName)
	}
	if resolved.Strategy != "conservative" {
		t.Errorf("expected conservative, got %s", resolved.Strategy)
	}
}

func TestIntResolver_StagingProfile(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{Profile: "staging"},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve staging failed: %v", err)
	}
	if resolved.ProfileName != "staging" {
		t.Errorf("expected staging, got %s", resolved.ProfileName)
	}
}

func TestIntResolver_DevelopmentProfile(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{Profile: "development"},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve development failed: %v", err)
	}
	if resolved.Strategy != "aggressive" {
		t.Errorf("expected aggressive, got %s", resolved.Strategy)
	}
}

func TestIntResolver_TestProfile(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{Profile: "test"},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve test failed: %v", err)
	}
	if resolved.ProfileName != "test" {
		t.Errorf("expected test, got %s", resolved.ProfileName)
	}
}

func TestIntResolver_InvalidProfile(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{Profile: "nonexistent"},
	}
	_, err := r.Resolve(cfg)
	if err == nil {
		t.Fatal("expected error for invalid profile")
	}
}

func TestIntResolver_WithProfileOverrides(t *testing.T) {
	r := profile.NewResolver()
	minConf := 30.0
	maxChange := 50.0
	dryRun := true
	requireApproval := false
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "production",
			ProfileOverrides: &optimizerv1alpha1.ProfileOverrides{
				MinConfidence:    &minConf,
				MaxChangePercent: &maxChange,
				RequireApproval:  &requireApproval,
				DryRun:           &dryRun,
				ApplyDelay:       "1h",
			},
		},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve with overrides failed: %v", err)
	}
	if resolved.MaxChangePercent != 50.0 {
		t.Errorf("expected MaxChangePercent=50, got %f", resolved.MaxChangePercent)
	}
}

func TestIntResolver_WithRecommendationOverrides(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "staging",
			Recommendations: &optimizerv1alpha1.RecommendationConfig{
				CPUPercentile:    85,
				MemoryPercentile: 85,
				SafetyMargin:     1.3,
				MinSamples:       150,
				HistoryDuration:  "48h",
			},
		},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve with recommendation overrides failed: %v", err)
	}
	if resolved.CPUPercentile != 85 {
		t.Errorf("expected CPUPercentile=85, got %d", resolved.CPUPercentile)
	}
	if resolved.SafetyMargin != 1.3 {
		t.Errorf("expected SafetyMargin=1.3, got %f", resolved.SafetyMargin)
	}
}

func TestIntResolver_WithCircuitBreakerOverride(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			CircuitBreaker: &optimizerv1alpha1.CircuitBreakerConfig{
				Enabled:        false,
				ErrorThreshold: 10,
			},
		},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve with circuit breaker failed: %v", err)
	}
	if resolved.CircuitBreakerEnabled {
		t.Error("expected circuit breaker disabled")
	}
	if resolved.CircuitBreakerThreshold != 10 {
		t.Errorf("expected threshold=10, got %d", resolved.CircuitBreakerThreshold)
	}
}

func TestIntResolver_WithResourceThresholds(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			ResourceThresholds: &optimizerv1alpha1.ResourceThresholds{
				CPU: &optimizerv1alpha1.ResourceLimit{
					Min: "200m",
					Max: "4000m",
				},
				Memory: &optimizerv1alpha1.ResourceLimit{
					Min: "256Mi",
					Max: "8Gi",
				},
			},
		},
	}
	resolved, err := r.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve with resource thresholds failed: %v", err)
	}
	if resolved.MinCPUMillicores != 200 {
		t.Errorf("expected MinCPU=200m, got %d", resolved.MinCPUMillicores)
	}
	if resolved.MaxCPUMillicores != 4000 {
		t.Errorf("expected MaxCPU=4000m, got %d", resolved.MaxCPUMillicores)
	}
}

func TestIntResolver_ShouldApplyRecommendation(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "development",
		},
	}
	resolved, _ := r.Resolve(cfg)

	ok, reason := resolved.ShouldApplyRecommendation(80.0, 20.0)
	if !ok {
		t.Errorf("expected apply=true for development, got false: %s", reason)
	}
}

func TestIntResolver_ShouldApply_DryRun(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{DryRun: true},
	}
	resolved, _ := r.Resolve(cfg)
	ok, _ := resolved.ShouldApplyRecommendation(100.0, 5.0)
	if ok {
		t.Error("dry-run mode should prevent apply")
	}
}

func TestIntResolver_ShouldApply_LowConfidence(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "production",
		},
	}
	resolved, _ := r.Resolve(cfg)
	ok, _ := resolved.ShouldApplyRecommendation(10.0, 5.0) // below min confidence
	if ok {
		t.Error("low confidence should prevent apply")
	}
}

func TestIntResolver_ShouldApply_TooMuchChange(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "production",
			DryRun:  false,
		},
	}
	resolved, _ := r.Resolve(cfg)
	// production has MaxChangePercent=20, try 50%
	ok, _ := resolved.ShouldApplyRecommendation(95.0, 50.0)
	if ok {
		t.Error("too much change should prevent apply")
	}
}

func TestIntResolver_ShouldApply_RequiresApproval(t *testing.T) {
	r := profile.NewResolver()
	cfg := &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Profile: "production",
			DryRun:  false,
		},
	}
	resolved, _ := r.Resolve(cfg)
	// production requires approval and DryRun overrides
	resolved.DryRun = false
	ok, _ := resolved.ShouldApplyRecommendation(100.0, 5.0)
	if ok {
		t.Error("manual approval required should prevent apply")
	}
}

func TestIntResolver_GetEffectiveStrategy(t *testing.T) {
	r := profile.NewResolver()
	cases := []struct {
		profile  string
		expected optimizerv1alpha1.OptimizationStrategy
	}{
		{"production", optimizerv1alpha1.StrategyConservative},
		{"staging", optimizerv1alpha1.StrategyBalanced},
		{"development", optimizerv1alpha1.StrategyAggressive},
	}
	for _, tc := range cases {
		cfg := &optimizerv1alpha1.OptimizerConfig{
			Spec: optimizerv1alpha1.OptimizerConfigSpec{Profile: optimizerv1alpha1.EnvironmentProfile(tc.profile)},
		}
		resolved, err := r.Resolve(cfg)
		if err != nil {
			t.Errorf("Resolve(%s) failed: %v", tc.profile, err)
			continue
		}
		got := resolved.GetEffectiveStrategy()
		if got != tc.expected {
			t.Errorf("profile=%s: expected %s, got %s", tc.profile, tc.expected, got)
		}
	}
}

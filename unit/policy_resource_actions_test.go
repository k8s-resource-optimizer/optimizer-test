package unit_test

import (
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/policy"
)

// makeResourceActionContext builds an EvaluationContext whose Recommendation
// has the given RecommendedCPU and RecommendedMemory values.
func makeResourceActionContext(recCPU, recMemory int64, changeType string) policy.EvaluationContext {
	return policy.EvaluationContext{
		Workload: policy.WorkloadInfo{
			Namespace: "default",
			Name:      "api",
			Kind:      "Deployment",
		},
		Recommendation: policy.RecommendationInfo{
			RecommendedCPU:    recCPU,
			RecommendedMemory: recMemory,
			ChangeType:        changeType,
			Confidence:        85.0,
		},
	}
}

// loadResourcePolicy returns an Engine loaded with a single resource-action policy.
func loadResourcePolicy(action, paramKey, paramValue string) (*policy.Engine, error) {
	data := []byte(`
policies:
  - name: resource-policy
    condition: 'true'
    action: ` + action + `
    parameters:
      ` + paramKey + `: ` + paramValue + `
    priority: 10
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	return e, e.LoadPoliciesFromBytes(data)
}

// ─── set-min-cpu tests ────────────────────────────────────────────────────────

// TestApplyAction_SetMinCPU_BelowMin verifies that when the recommended CPU is
// below the policy minimum the decision modifies (increases) it.
func TestApplyAction_SetMinCPU_BelowMin(t *testing.T) {
	e, err := loadResourcePolicy("set-min-cpu", "min-cpu", "500m")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	// Recommended CPU (200m) is below minimum (500m) → should be bumped up.
	ctx := makeResourceActionContext(200, 256*1024*1024, "scaledown")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != "modify" {
		t.Errorf("expected action='modify' when CPU below min, got %q", result.Action)
	}
}

// TestApplyAction_SetMinCPU_AboveMin verifies that when the recommended CPU is
// already above the minimum no modification is made.
func TestApplyAction_SetMinCPU_AboveMin(t *testing.T) {
	e, err := loadResourcePolicy("set-min-cpu", "min-cpu", "100m")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(800, 256*1024*1024, "nochange")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != policy.ActionAllow {
		t.Errorf("expected action='allow' when CPU above min, got %q", result.Action)
	}
}

// ─── set-max-cpu tests ────────────────────────────────────────────────────────

// TestApplyAction_SetMaxCPU_AboveMax verifies that when the recommended CPU
// exceeds the policy maximum it is capped (modified down).
func TestApplyAction_SetMaxCPU_AboveMax(t *testing.T) {
	e, err := loadResourcePolicy("set-max-cpu", "max-cpu", "500m")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(800, 256*1024*1024, "scaleup")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != "modify" {
		t.Errorf("expected action='modify' when CPU above max, got %q", result.Action)
	}
}

// TestApplyAction_SetMaxCPU_BelowMax verifies that CPU under the max is allowed.
func TestApplyAction_SetMaxCPU_BelowMax(t *testing.T) {
	e, err := loadResourcePolicy("set-max-cpu", "max-cpu", "2")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(500, 256*1024*1024, "nochange")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != policy.ActionAllow {
		t.Errorf("expected action='allow' when CPU below max, got %q", result.Action)
	}
}

// ─── set-min-memory tests ─────────────────────────────────────────────────────

// TestApplyAction_SetMinMemory_BelowMin verifies that memory below the minimum
// triggers a modify decision.
func TestApplyAction_SetMinMemory_BelowMin(t *testing.T) {
	e, err := loadResourcePolicy("set-min-memory", "min-memory", "256Mi")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(500, 64*1024*1024, "scaledown") // 64Mi < 256Mi
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != "modify" {
		t.Errorf("expected action='modify' when memory below min, got %q", result.Action)
	}
}

// TestApplyAction_SetMinMemory_ModificationMessageNotEmpty verifies that the
// modify decision includes a non-empty modifications list.
func TestApplyAction_SetMinMemory_ModificationMessageNotEmpty(t *testing.T) {
	e, err := loadResourcePolicy("set-min-memory", "min-memory", "512Mi")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(500, 128*1024*1024, "scaledown")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action == "modify" && result.ModifiedRecommendation != nil {
		if len(result.ModifiedRecommendation.Modifications) == 0 {
			t.Error("expected non-empty Modifications slice in modify decision")
		}
	}
}

// ─── set-max-memory tests ─────────────────────────────────────────────────────

// TestApplyAction_SetMaxMemory_AboveMax verifies that memory above the maximum
// is capped.
func TestApplyAction_SetMaxMemory_AboveMax(t *testing.T) {
	e, err := loadResourcePolicy("set-max-memory", "max-memory", "256Mi")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(500, 1024*1024*1024, "scaleup") // 1Gi > 256Mi
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != "modify" {
		t.Errorf("expected action='modify' when memory above max, got %q", result.Action)
	}
}

// ─── skip-scale-down / skip-scale-up tests ────────────────────────────────────

// TestApplyAction_SkipScaleDown_BlocksWhenScalingDown verifies that
// skip-scale-down denies a scale-down change type.
func TestApplyAction_SkipScaleDown_BlocksWhenScalingDown(t *testing.T) {
	data := []byte(`
policies:
  - name: no-scaledown
    condition: 'true'
    action: skip-scaledown
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(400, 256*1024*1024, "scaledown")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != policy.ActionDeny {
		t.Errorf("expected deny for scaledown with skip-scale-down policy, got %q", result.Action)
	}
}

// TestApplyAction_SkipScaleDown_AllowsWhenNotScalingDown verifies that
// skip-scale-down allows non-scale-down changes.
func TestApplyAction_SkipScaleDown_AllowsWhenNotScalingDown(t *testing.T) {
	data := []byte(`
policies:
  - name: no-scaledown
    condition: 'true'
    action: skip-scaledown
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(800, 512*1024*1024, "scaleup")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != policy.ActionAllow {
		t.Errorf("expected allow for scaleup with skip-scale-down policy, got %q", result.Action)
	}
}

// TestApplyAction_SkipScaleUp_BlocksWhenScalingUp verifies that skip-scale-up
// denies a scale-up recommendation.
func TestApplyAction_SkipScaleUp_BlocksWhenScalingUp(t *testing.T) {
	data := []byte(`
policies:
  - name: no-scaleup
    condition: 'true'
    action: skip-scaleup
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(1200, 768*1024*1024, "scaleup")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action != policy.ActionDeny {
		t.Errorf("expected deny for scaleup with skip-scale-up policy, got %q", result.Action)
	}
}

// ─── validateActionParameters tests ─────────────────────────────────────────

// TestValidateActionParameters_MissingMinCPUParam verifies that loading a
// set-min-cpu policy without the required parameter fails.
func TestValidateActionParameters_MissingMinCPUParam(t *testing.T) {
	data := []byte(`
policies:
  - name: broken-cpu
    condition: 'true'
    action: set-min-cpu
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	if err := e.LoadPoliciesFromBytes(data); err == nil {
		t.Error("expected error for set-min-cpu without min-cpu parameter")
	}
}

// TestValidateActionParameters_MissingMaxMemoryParam verifies that loading a
// set-max-memory policy without the required parameter fails.
func TestValidateActionParameters_MissingMaxMemoryParam(t *testing.T) {
	data := []byte(`
policies:
  - name: broken-mem
    condition: 'true'
    action: set-max-memory
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	if err := e.LoadPoliciesFromBytes(data); err == nil {
		t.Error("expected error for set-max-memory without max-memory parameter")
	}
}

// TestModifiedRecommendation_MemoryRequestSet verifies that after a memory-max
// modification, MemoryRequest in the result points to the capped value.
func TestModifiedRecommendation_MemoryRequestSet(t *testing.T) {
	e, err := loadResourcePolicy("set-max-memory", "max-memory", "256Mi")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(500, 512*1024*1024, "scaleup")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action == "modify" && result.ModifiedRecommendation != nil {
		if result.ModifiedRecommendation.MemoryRequest == nil {
			t.Error("expected MemoryRequest to be set in modified recommendation")
		} else {
			expected := int64(256 * 1024 * 1024)
			if *result.ModifiedRecommendation.MemoryRequest != expected {
				t.Errorf("expected MemoryRequest=%d (256Mi), got %d",
					expected, *result.ModifiedRecommendation.MemoryRequest)
			}
		}
		if len(result.ModifiedRecommendation.Modifications) == 0 {
			t.Error("expected non-empty Modifications list")
		}
	}
}

// TestModifiedRecommendation_CPURequestSet verifies that after a CPU-min
// modification, CPURequest points to the enforced minimum.
func TestModifiedRecommendation_CPURequestSet(t *testing.T) {
	e, err := loadResourcePolicy("set-min-cpu", "min-cpu", "300m")
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}
	ctx := makeResourceActionContext(100, 256*1024*1024, "scaledown")
	result, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if result.Action == "modify" && result.ModifiedRecommendation != nil {
		if result.ModifiedRecommendation.CPURequest == nil {
			t.Error("expected CPURequest to be set in modified recommendation")
		}
		if !strings.Contains(result.ModifiedRecommendation.Modifications[0], "Increased CPU") {
			t.Errorf("expected modification message about CPU increase, got: %v",
				result.ModifiedRecommendation.Modifications)
		}
	}
}

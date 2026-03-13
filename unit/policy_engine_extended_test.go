package unit_test

import (
	"testing"

	"intelligent-cluster-optimizer/pkg/policy"
)

// ─── Engine accessor methods tests ──────────────────────────────────────────

// TestPolicyEngine_GetPolicies_EmptyBeforeLoad verifies that a fresh engine
// has an empty policies list.
func TestPolicyEngine_GetPolicies_EmptyBeforeLoad(t *testing.T) {
	e := policy.NewEngine()
	policies := e.GetPolicies()
	if len(policies) != 0 {
		t.Errorf("expected 0 policies in new engine, got %d", len(policies))
	}
}

// TestPolicyEngine_GetPolicies_AfterLoad verifies that policies loaded via
// LoadPoliciesFromBytes are returned by GetPolicies.
func TestPolicyEngine_GetPolicies_AfterLoad(t *testing.T) {
	e := policy.NewEngine()
	data := []byte(`
policies:
  - name: allow-all
    condition: 'true'
    action: allow
    priority: 1
    enabled: true
  - name: deny-prod
    condition: 'workload.namespace == "production"'
    action: deny
    priority: 10
    enabled: true
defaultAction: allow
`)
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}

	policies := e.GetPolicies()
	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(policies))
	}
}

// TestPolicyEngine_GetDefaultAction_Allow verifies that the default action
// is returned correctly when set to "allow".
func TestPolicyEngine_GetDefaultAction_Allow(t *testing.T) {
	e := policy.NewEngine()
	data := []byte(`
policies: []
defaultAction: allow
`)
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}

	action := e.GetDefaultAction()
	if action != "allow" {
		t.Errorf("expected default action 'allow', got %q", action)
	}
}

// TestPolicyEngine_GetDefaultAction_Deny verifies that the default action
// is returned correctly when set to "deny".
func TestPolicyEngine_GetDefaultAction_Deny(t *testing.T) {
	e := policy.NewEngine()
	data := []byte(`
policies: []
defaultAction: deny
`)
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}

	action := e.GetDefaultAction()
	if action != "deny" {
		t.Errorf("expected default action 'deny', got %q", action)
	}
}

// TestPolicyEngine_ClearCache_NoPanic verifies that ClearCache does not panic
// on a freshly created engine or after loading policies.
func TestPolicyEngine_ClearCache_NoPanic(t *testing.T) {
	e := policy.NewEngine()
	e.ClearCache() // on fresh engine

	data := []byte(`
policies:
  - name: allow-all
    condition: 'true'
    action: allow
    priority: 1
    enabled: true
defaultAction: allow
`)
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}

	// Evaluate to populate the cache, then clear it.
	ctx := makeContext("default", "my-app", "scale-down", -30, -20)
	_, _ = e.Evaluate(ctx)
	e.ClearCache() // must not panic
}

// TestPolicyEngine_ClearCache_ReEvaluateAfterClear verifies that evaluation
// still works correctly after clearing the cache.
func TestPolicyEngine_ClearCache_ReEvaluateAfterClear(t *testing.T) {
	e := policy.NewEngine()
	data := []byte(`
policies:
  - name: allow-all
    condition: 'true'
    action: allow
    priority: 5
    enabled: true
defaultAction: deny
`)
	if err := e.LoadPoliciesFromBytes(data); err != nil {
		t.Fatalf("LoadPoliciesFromBytes error: %v", err)
	}

	ctx := makeContext("default", "worker", "scale-down", -20, -15)
	result1, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate before clear error: %v", err)
	}

	e.ClearCache()

	result2, err := e.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate after clear error: %v", err)
	}

	if result1.Action != result2.Action {
		t.Errorf("expected same action before/after cache clear, got %q vs %q",
			result1.Action, result2.Action)
	}
}

package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/policy"
)

// makePolicyYAML builds a minimal YAML policy document with a single rule.
// Single quotes wrap the condition so that embedded double quotes (common in
// expr-lang expressions like `workload.namespace == "production"`) are valid YAML.
func makePolicyYAML(name, condition, action string) []byte {
	return []byte(`
policies:
  - name: ` + name + `
    condition: '` + condition + `'
    action: ` + action + `
    priority: 10
    enabled: true
defaultAction: allow
`)
}

// makeContext returns a fully populated EvaluationContext.
// Adjust the fields to match the scenario you are testing.
func makeContext(ns, workload, changeType string, cpuChange, memChange float64) policy.EvaluationContext {
	return policy.EvaluationContext{
		Workload: policy.WorkloadInfo{
			Namespace:     ns,
			Name:          workload,
			Kind:          "Deployment",
			Labels:        map[string]string{"env": "production", "app-type": "web"},
			Annotations:   map[string]string{},
			Replicas:      3,
			CurrentCPU:    500,
			CurrentMemory: 256 * 1024 * 1024,
		},
		Recommendation: policy.RecommendationInfo{
			RecommendedCPU:      600,
			RecommendedMemory:   300 * 1024 * 1024,
			Confidence:          90.0,
			ChangeType:          changeType,
			CPUChangePercent:    cpuChange,
			MemoryChangePercent: memChange,
		},
		Time: policy.TimeInfo{
			Now:             time.Now(),
			Hour:            14,
			Weekday:         2,
			IsBusinessHours: true,
			IsWeekend:       false,
		},
		Cluster: policy.ClusterInfo{
			TotalNodes:  5,
			Environment: "production",
		},
	}
}

// TestPolicyEngine_ActionAllow verifies that the "allow" action passes
// a recommendation through without modification.
func TestPolicyEngine_ActionAllow(t *testing.T) {
	eng := policy.NewEngine()
	// The condition always evaluates to true (literal `true`).
	err := eng.LoadPoliciesFromBytes(makePolicyYAML("always-allow", "true", "allow"))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("default", "api", "scaleup", 20.0, 17.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != policy.ActionAllow {
		t.Errorf("expected action %q, got %q", policy.ActionAllow, decision.Action)
	}
}

// TestPolicyEngine_ActionDeny verifies that a "deny" policy blocks
// a recommendation that matches its condition.
func TestPolicyEngine_ActionDeny(t *testing.T) {
	eng := policy.NewEngine()
	// Deny any change where the workload is in the "production" namespace.
	err := eng.LoadPoliciesFromBytes(makePolicyYAML(
		"deny-production",
		`workload.namespace == "production"`,
		"deny",
	))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("production", "api", "scaledown", -15.0, -10.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != policy.ActionDeny {
		t.Errorf("expected action %q, got %q", policy.ActionDeny, decision.Action)
	}
}

// TestPolicyEngine_ActionSkip verifies that the "skip" action is handled.
//
// Implementation note: applyAction() maps ActionSkip → ActionDeny in the
// returned PolicyDecision.  "Skip" means "do not apply this recommendation",
// which is semantically equivalent to a deny.  The test therefore asserts
// that the returned action is ActionDeny (not ActionSkip), matching the
// documented source behaviour in engine_test.go.
func TestPolicyEngine_ActionSkip(t *testing.T) {
	eng := policy.NewEngine()
	err := eng.LoadPoliciesFromBytes(makePolicyYAML(
		"skip-low-confidence",
		`recommendation.confidence < 50`,
		"skip",
	))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("default", "job", "scaledown", -20.0, -15.0)
	ctx.Recommendation.Confidence = 30 // below 50 → triggers skip
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// "skip" maps to "deny" internally — this is intentional by design.
	if decision.Action != policy.ActionDeny {
		t.Errorf("expected skip to produce ActionDeny, got %q", decision.Action)
	}
}

// TestPolicyEngine_ActionRequireApproval verifies that the require-approval
// action is correctly returned when the condition matches.
func TestPolicyEngine_ActionRequireApproval(t *testing.T) {
	eng := policy.NewEngine()
	// Any scale-up exceeding 30% CPU change requires manual approval.
	err := eng.LoadPoliciesFromBytes(makePolicyYAML(
		"large-change-approval",
		`recommendation.cpuChangePercent > 30`,
		"require-approval",
	))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("staging", "backend", "scaleup", 50.0, 30.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != policy.ActionRequireApproval {
		t.Errorf("expected action %q, got %q", policy.ActionRequireApproval, decision.Action)
	}
}

// TestPolicyEngine_DefaultActionWhenNoMatch verifies that when no policy
// matches the context, the defaultAction from the policy file is used.
func TestPolicyEngine_DefaultActionWhenNoMatch(t *testing.T) {
	eng := policy.NewEngine()
	// The condition will never match (requires namespace "neverexists").
	err := eng.LoadPoliciesFromBytes(makePolicyYAML(
		"unreachable",
		`workload.namespace == "neverexists"`,
		"deny",
	))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("default", "frontend", "scaleup", 10.0, 5.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Default action in the YAML is "allow".
	if decision.Action != policy.ActionAllow {
		t.Errorf("expected default action %q, got %q", policy.ActionAllow, decision.Action)
	}
}

// TestPolicyEngine_PriorityOrder verifies that higher-priority policies
// are evaluated first and their decision wins.
func TestPolicyEngine_PriorityOrder(t *testing.T) {
	eng := policy.NewEngine()
	// Two overlapping policies: deny at priority 10, allow at priority 5.
	// The deny policy wins because it has higher priority.
	err := eng.LoadPoliciesFromBytes([]byte(`
policies:
  - name: deny-high-priority
    condition: "true"
    action: deny
    priority: 10
    enabled: true
  - name: allow-low-priority
    condition: "true"
    action: allow
    priority: 5
    enabled: true
defaultAction: allow
`))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("default", "api", "scaleup", 10.0, 5.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != policy.ActionDeny {
		t.Errorf("higher-priority deny should win, got %q", decision.Action)
	}
}

// TestPolicyEngine_EvaluationWithin50ms verifies the latency requirement:
// policy evaluation must complete within 50 milliseconds.
// This is directly stated in the project spec.
func TestPolicyEngine_EvaluationWithin50ms(t *testing.T) {
	eng := policy.NewEngine()
	// Load a realistic set of rules to stress-test evaluation time.
	err := eng.LoadPoliciesFromBytes([]byte(`
policies:
  - name: check-namespace
    condition: "workload.namespace == \"production\""
    action: deny
    priority: 100
    enabled: true
  - name: check-confidence
    condition: "recommendation.confidence < 70"
    action: skip
    priority: 90
    enabled: true
  - name: check-large-scaleup
    condition: "recommendation.cpuChangePercent > 50"
    action: require-approval
    priority: 80
    enabled: true
  - name: check-weekend
    condition: "time.isWeekend"
    action: skip
    priority: 70
    enabled: true
  - name: allow-rest
    condition: "true"
    action: allow
    priority: 1
    enabled: true
defaultAction: allow
`))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("staging", "service", "scaleup", 20.0, 10.0)

	// Warm-up run to ensure compilation overhead is excluded from the measurement.
	_, _ = eng.Evaluate(ctx)

	start := time.Now()
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		_, err := eng.Evaluate(ctx)
		if err != nil {
			t.Fatalf("Evaluate iteration %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	avgPerCall := elapsed / iterations

	if avgPerCall > 50*time.Millisecond {
		t.Errorf("policy evaluation too slow: avg=%v (must be <50ms per call)", avgPerCall)
	} else {
		t.Logf("policy evaluation avg latency: %v", avgPerCall)
	}
}

// TestPolicyEngine_InvalidYAMLReturnsError verifies that malformed policy
// YAML produces a descriptive error rather than silently using defaults.
func TestPolicyEngine_InvalidYAMLReturnsError(t *testing.T) {
	eng := policy.NewEngine()
	err := eng.LoadPoliciesFromBytes([]byte(`not: valid: yaml: [[[`))
	if err == nil {
		t.Error("expected an error for invalid YAML, got nil")
	}
}

// TestPolicyEngine_DisabledPolicyIsSkipped verifies that policies with
// enabled=false are not evaluated.
func TestPolicyEngine_DisabledPolicyIsSkipped(t *testing.T) {
	eng := policy.NewEngine()
	err := eng.LoadPoliciesFromBytes([]byte(`
policies:
  - name: disabled-deny
    condition: "true"
    action: deny
    priority: 10
    enabled: false
defaultAction: allow
`))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("default", "app", "scaleup", 10.0, 5.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Disabled deny should be skipped; default action (allow) takes effect.
	if decision.Action != policy.ActionAllow {
		t.Errorf("disabled policy should be skipped, expected allow, got %q", decision.Action)
	}
}

// TestPolicyEngine_MatchedPolicyReported verifies that PolicyDecision.MatchedPolicy
// is populated with the name of the rule that triggered the decision.
func TestPolicyEngine_MatchedPolicyReported(t *testing.T) {
	eng := policy.NewEngine()
	err := eng.LoadPoliciesFromBytes(makePolicyYAML("my-deny-rule", "true", "deny"))
	if err != nil {
		t.Fatalf("LoadPoliciesFromBytes: %v", err)
	}

	ctx := makeContext("default", "svc", "scaleup", 5.0, 5.0)
	decision, err := eng.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.MatchedPolicy != "my-deny-rule" {
		t.Errorf("expected MatchedPolicy=%q, got %q", "my-deny-rule", decision.MatchedPolicy)
	}
}

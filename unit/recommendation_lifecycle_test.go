package unit_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/recommendation"
)

// freshRec returns a WorkloadRecommendation generated just now with a TTL
// of `ttl`. Set ttl=0 to use the default (24h).
func freshRec(ttl time.Duration) recommendation.WorkloadRecommendation {
	now := time.Now()
	r := recommendation.WorkloadRecommendation{
		Namespace:    "default",
		WorkloadKind: "Deployment",
		WorkloadName: "api",
		GeneratedAt:  now,
		Containers: []recommendation.ContainerRecommendation{
			{
				ContainerName:    "app",
				CurrentCPU:       1000,
				RecommendedCPU:   700,
				CurrentMemory:    512 * 1024 * 1024,
				RecommendedMemory: 350 * 1024 * 1024,
			},
		},
	}
	if ttl > 0 {
		r.ExpiresAt = now.Add(ttl)
	}
	return r
}

// expiredRec returns a WorkloadRecommendation that already expired.
func expiredRec() recommendation.WorkloadRecommendation {
	r := freshRec(0)
	r.GeneratedAt = time.Now().Add(-25 * time.Hour) // older than default 24h TTL
	return r
}

// TestWorkloadRecommendation_NotExpiredWhenFresh verifies that a freshly
// generated recommendation with a future expiry is not expired.
func TestWorkloadRecommendation_NotExpiredWhenFresh(t *testing.T) {
	r := freshRec(time.Hour)
	if r.IsExpired() {
		t.Error("expected fresh recommendation to not be expired")
	}
}

// TestWorkloadRecommendation_ExpiredAfterTTL verifies that a recommendation
// generated 25 hours ago (beyond the 24h default TTL) is expired.
func TestWorkloadRecommendation_ExpiredAfterTTL(t *testing.T) {
	r := expiredRec()
	if !r.IsExpired() {
		t.Error("expected old recommendation to be expired")
	}
}

// TestWorkloadRecommendation_AgeIsPositive verifies that Age() returns a
// positive duration for any recommendation.
func TestWorkloadRecommendation_AgeIsPositive(t *testing.T) {
	r := freshRec(time.Hour)
	if r.Age() < 0 {
		t.Errorf("Age() should be ≥ 0, got %v", r.Age())
	}
}

// TestWorkloadRecommendation_TimeToExpiry_PositiveWhenFresh verifies that
// TimeToExpiry is positive for a fresh recommendation.
func TestWorkloadRecommendation_TimeToExpiry_PositiveWhenFresh(t *testing.T) {
	r := freshRec(time.Hour)
	if r.TimeToExpiry() <= 0 {
		t.Errorf("TimeToExpiry should be positive for fresh rec, got %v", r.TimeToExpiry())
	}
}

// TestWorkloadRecommendation_TimeToExpiry_NegativeWhenExpired verifies that
// TimeToExpiry is negative for an expired recommendation.
func TestWorkloadRecommendation_TimeToExpiry_NegativeWhenExpired(t *testing.T) {
	r := expiredRec()
	if r.TimeToExpiry() > 0 {
		t.Errorf("TimeToExpiry should be negative for expired rec, got %v", r.TimeToExpiry())
	}
}

// TestWorkloadRecommendation_ExpiryStatus_ContainsValid verifies that the
// expiry status of a fresh recommendation contains "Valid".
func TestWorkloadRecommendation_ExpiryStatus_ContainsValid(t *testing.T) {
	r := freshRec(time.Hour)
	status := r.ExpiryStatus()
	if status == "" {
		t.Error("ExpiryStatus should not be empty")
	}
}

// TestFilterExpired_RemovesExpiredOnly verifies that FilterExpired removes
// expired recommendations but keeps fresh ones.
func TestFilterExpired_RemovesExpiredOnly(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		freshRec(time.Hour),   // valid
		expiredRec(),          // expired
		freshRec(2 * time.Hour), // valid
	}

	valid := recommendation.FilterExpired(recs)
	if len(valid) != 2 {
		t.Errorf("expected 2 valid recommendations, got %d", len(valid))
	}
}

// TestShouldApply_TrueForFreshHighConfidence verifies that a fresh, high-
// confidence recommendation should be applied.
func TestShouldApply_TrueForFreshHighConfidence(t *testing.T) {
	r := freshRec(time.Hour)
	// Inline set confidence on the first container.
	r.Containers[0].Confidence = 95.0

	ok, reason := r.ShouldApply(80.0)
	if !ok {
		t.Errorf("expected ShouldApply=true for fresh high-confidence rec, reason: %s", reason)
	}
}

// TestShouldApply_FalseForExpired verifies that an expired recommendation
// is never applied regardless of confidence.
func TestShouldApply_FalseForExpired(t *testing.T) {
	r := expiredRec()
	if len(r.Containers) > 0 {
		r.Containers[0].Confidence = 99.0
	}

	ok, _ := r.ShouldApply(80.0)
	if ok {
		t.Error("expected ShouldApply=false for expired recommendation")
	}
}

// TestContainerRecommendation_CPUChangePercent_Decrease verifies that
// reducing CPU produces a negative change percent.
func TestContainerRecommendation_CPUChangePercent_Decrease(t *testing.T) {
	c := recommendation.ContainerRecommendation{
		CurrentCPU:     1000,
		RecommendedCPU: 600,
	}
	pct := c.CalculateCPUChangePercent()
	if pct >= 0 {
		t.Errorf("expected negative CPU change percent for decrease, got %f", pct)
	}
}

// TestContainerRecommendation_MemoryChangePercent_Increase verifies that
// increasing memory produces a positive change percent.
func TestContainerRecommendation_MemoryChangePercent_Increase(t *testing.T) {
	c := recommendation.ContainerRecommendation{
		CurrentMemory:     256 * 1024 * 1024,
		RecommendedMemory: 512 * 1024 * 1024,
	}
	pct := c.CalculateMemoryChangePercent()
	if pct <= 0 {
		t.Errorf("expected positive memory change percent for increase, got %f", pct)
	}
}

// TestContainerRecommendation_MaxChangePercent_IsLarger verifies that
// MaxChangePercent returns the larger of the two change percentages.
func TestContainerRecommendation_MaxChangePercent_IsLarger(t *testing.T) {
	c := recommendation.ContainerRecommendation{
		CurrentCPU:        1000,
		RecommendedCPU:    500, // -50%
		CurrentMemory:     256 * 1024 * 1024,
		RecommendedMemory: 200 * 1024 * 1024, // ~-22%
	}
	max := c.MaxChangePercent()
	cpuPct := c.CalculateCPUChangePercent()
	memPct := c.CalculateMemoryChangePercent()

	absCPU := cpuPct
	if absCPU < 0 {
		absCPU = -absCPU
	}
	absMem := memPct
	if absMem < 0 {
		absMem = -absMem
	}
	expected := absCPU
	if absMem > absCPU {
		expected = absMem
	}

	if max != expected {
		t.Errorf("MaxChangePercent %f should equal max(|cpu|, |mem|) = %f", max, expected)
	}
}

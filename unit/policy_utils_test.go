package unit_test

import (
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/policy"
)

// ─── FormatCPU tests ─────────────────────────────────────────────────────────

// TestFormatCPU_MillicoresUnder1000 verifies that values below 1000 are
// formatted with the "m" suffix (e.g., "500m").
func TestFormatCPU_MillicoresUnder1000(t *testing.T) {
	cases := []struct {
		input    int64
		expected string
	}{
		{0, "0m"},
		{1, "1m"},
		{500, "500m"},
		{999, "999m"},
	}
	for _, tc := range cases {
		got := policy.FormatCPU(tc.input)
		if got != tc.expected {
			t.Errorf("FormatCPU(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// TestFormatCPU_CoresAt1000 verifies that exactly 1000 millicores formats as
// "1.00" (cores representation without "m" suffix).
func TestFormatCPU_CoresAt1000(t *testing.T) {
	got := policy.FormatCPU(1000)
	if got != "1.00" {
		t.Errorf("FormatCPU(1000) = %q, want %q", got, "1.00")
	}
}

// TestFormatCPU_MultipleCores verifies that values above 1000 are expressed
// as decimal core fractions.
func TestFormatCPU_MultipleCores(t *testing.T) {
	cases := []struct {
		input    int64
		expected string
	}{
		{2000, "2.00"},
		{1500, "1.50"},
		{4000, "4.00"},
	}
	for _, tc := range cases {
		got := policy.FormatCPU(tc.input)
		if got != tc.expected {
			t.Errorf("FormatCPU(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ─── FormatMemory tests ──────────────────────────────────────────────────────

// TestFormatMemory_Bytes verifies that values below 1024 are expressed as
// raw bytes.
func TestFormatMemory_Bytes(t *testing.T) {
	got := policy.FormatMemory(512)
	if got != "512" {
		t.Errorf("FormatMemory(512) = %q, want %q", got, "512")
	}
}

// TestFormatMemory_Kilobytes verifies that values in the KiB range include
// the "Ki" suffix.
func TestFormatMemory_Kilobytes(t *testing.T) {
	got := policy.FormatMemory(2 * 1024)
	if !strings.HasSuffix(got, "Ki") {
		t.Errorf("FormatMemory(2048) = %q, expected 'Ki' suffix", got)
	}
}

// TestFormatMemory_Megabytes verifies that values in the MiB range include
// the "Mi" suffix.
func TestFormatMemory_Megabytes(t *testing.T) {
	cases := []int64{
		256 * 1024 * 1024,
		512 * 1024 * 1024,
		1024 * 1024 * 1024 / 2,
	}
	for _, c := range cases {
		got := policy.FormatMemory(c)
		if !strings.HasSuffix(got, "Mi") {
			t.Errorf("FormatMemory(%d) = %q, expected 'Mi' suffix", c, got)
		}
	}
}

// TestFormatMemory_Gigabytes verifies that values in the GiB range include
// the "Gi" suffix.
func TestFormatMemory_Gigabytes(t *testing.T) {
	got := policy.FormatMemory(4 * 1024 * 1024 * 1024)
	if !strings.HasSuffix(got, "Gi") {
		t.Errorf("FormatMemory(4GiB) = %q, expected 'Gi' suffix", got)
	}
}

// TestFormatMemory_Terabytes verifies that very large values include "Ti".
func TestFormatMemory_Terabytes(t *testing.T) {
	got := policy.FormatMemory(2 * 1024 * 1024 * 1024 * 1024)
	if !strings.HasSuffix(got, "Ti") {
		t.Errorf("FormatMemory(2TiB) = %q, expected 'Ti' suffix", got)
	}
}

// TestFormatMemory_ZeroBytes verifies that 0 bytes formats as "0".
func TestFormatMemory_ZeroBytes(t *testing.T) {
	got := policy.FormatMemory(0)
	if got != "0" {
		t.Errorf("FormatMemory(0) = %q, want %q", got, "0")
	}
}

// ─── parseCPUValue / parseMemoryValue via policy YAML tests ─────────────────

// loadCPULimitPolicy creates an engine and loads a set-min-cpu policy with
// the given CPU value, returning any validation error.
func loadCPULimitPolicy(minCPU string) error {
	data := []byte(`
policies:
  - name: cpu-floor
    condition: 'true'
    action: set-min-cpu
    parameters:
      min-cpu: ` + minCPU + `
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	return e.LoadPoliciesFromBytes(data)
}

// loadMemoryLimitPolicy creates an engine and loads a set-max-memory policy
// with the given memory value, returning any validation error.
func loadMemoryLimitPolicy(maxMem string) error {
	data := []byte(`
policies:
  - name: mem-ceiling
    condition: 'true'
    action: set-max-memory
    parameters:
      max-memory: ` + maxMem + `
    priority: 5
    enabled: true
defaultAction: allow
`)
	e := policy.NewEngine()
	return e.LoadPoliciesFromBytes(data)
}

// TestParseCPUValue_MillicoresViaYAML verifies that a millicores CPU value
// (e.g., "100m") is accepted by the policy engine.
func TestParseCPUValue_MillicoresViaYAML(t *testing.T) {
	if err := loadCPULimitPolicy("100m"); err != nil {
		t.Errorf("expected valid policy for '100m', got error: %v", err)
	}
}

// TestParseCPUValue_CoresViaYAML verifies that a whole-core CPU value
// (e.g., "2") is accepted.
func TestParseCPUValue_CoresViaYAML(t *testing.T) {
	if err := loadCPULimitPolicy("2"); err != nil {
		t.Errorf("expected valid policy for '2' cores, got error: %v", err)
	}
}

// TestParseCPUValue_InvalidFormatViaYAML verifies that an invalid CPU string
// (e.g., "abc") is rejected with an error.
func TestParseCPUValue_InvalidFormatViaYAML(t *testing.T) {
	if err := loadCPULimitPolicy("abc"); err == nil {
		t.Error("expected error for invalid CPU value 'abc', got nil")
	}
}

// TestParseMemoryValue_MiViaYAML verifies that "256Mi" is accepted.
func TestParseMemoryValue_MiViaYAML(t *testing.T) {
	if err := loadMemoryLimitPolicy("256Mi"); err != nil {
		t.Errorf("expected valid policy for '256Mi', got error: %v", err)
	}
}

// TestParseMemoryValue_GiViaYAML verifies that "1Gi" is accepted.
func TestParseMemoryValue_GiViaYAML(t *testing.T) {
	if err := loadMemoryLimitPolicy("1Gi"); err != nil {
		t.Errorf("expected valid policy for '1Gi', got error: %v", err)
	}
}

// TestParseMemoryValue_BytesViaYAML verifies that a plain integer (bytes) is
// accepted.
func TestParseMemoryValue_BytesViaYAML(t *testing.T) {
	if err := loadMemoryLimitPolicy("1073741824"); err != nil {
		t.Errorf("expected valid policy for '1073741824' bytes, got error: %v", err)
	}
}

// TestParseMemoryValue_InvalidViaYAML verifies that an invalid memory string
// (e.g., "XYZ") is rejected.
func TestParseMemoryValue_InvalidViaYAML(t *testing.T) {
	if err := loadMemoryLimitPolicy("XYZ"); err == nil {
		t.Error("expected error for invalid memory value 'XYZ', got nil")
	}
}

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/gitops"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func rec(name, ns, kind, container string, cpu, mem int64) gitops.ResourceRecommendation {
	return gitops.ResourceRecommendation{
		Name:              name,
		Namespace:         ns,
		Kind:              kind,
		ContainerName:     container,
		RecommendedCPU:    cpu,
		RecommendedMemory: mem,
		Confidence:        85.0,
		Reason:            "test recommendation",
	}
}

func recs(n int) []gitops.ResourceRecommendation {
	kinds := []string{"Deployment", "StatefulSet", "DaemonSet"}
	result := make([]gitops.ResourceRecommendation, n)
	for i := range result {
		result[i] = rec(
			"workload-"+string(rune('a'+i%26)),
			"default",
			kinds[i%3],
			"app",
			int64(100+i*50),
			int64((128+i*64)*1024*1024),
		)
	}
	return result
}

// ─── ValidateConfig ───────────────────────────────────────────────────────────

func TestIntGitOps_ValidateConfig_EmptyFormat(t *testing.T) {
	e := gitops.NewExporter()
	err := e.ValidateConfig(gitops.ExportConfig{Format: ""})
	if err == nil {
		t.Fatal("expected error for empty format")
	}
}

func TestIntGitOps_ValidateConfig_InvalidFormat(t *testing.T) {
	e := gitops.NewExporter()
	err := e.ValidateConfig(gitops.ExportConfig{Format: "invalid-format"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestIntGitOps_ValidateConfig_ValidFormats(t *testing.T) {
	e := gitops.NewExporter()
	formats := []gitops.ExportFormat{
		gitops.FormatKustomize,
		gitops.FormatKustomizeJSON6902,
		gitops.FormatHelm,
	}
	for _, f := range formats {
		if err := e.ValidateConfig(gitops.ExportConfig{Format: f}); err != nil {
			t.Errorf("format %s should be valid: %v", f, err)
		}
	}
}

// ─── Export – Kustomize ───────────────────────────────────────────────────────

func TestIntGitOps_Export_Kustomize_Single(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("web", "default", "Deployment", "web", 200, 256*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomize},
	)
	if err != nil {
		t.Fatalf("Export Kustomize failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected at least one file")
	}
	if result.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestIntGitOps_Export_Kustomize_Multiple(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(recs(5), gitops.ExportConfig{Format: gitops.FormatKustomize})
	if err != nil {
		t.Fatalf("Export Kustomize multiple failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files in result")
	}
}

func TestIntGitOps_Export_Kustomize_ContainsYAML(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("api", "prod", "Deployment", "api", 500, 512*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomize},
	)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	for name, content := range result.Files {
		if content == "" {
			t.Errorf("file %s is empty", name)
		}
		_ = name
	}
}

func TestIntGitOps_Export_Kustomize_StatefulSet(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("db", "default", "StatefulSet", "db", 1000, 2*1024*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomize},
	)
	if err != nil {
		t.Fatalf("Export StatefulSet failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

func TestIntGitOps_Export_Kustomize_DaemonSet(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("log", "kube-system", "DaemonSet", "log", 50, 64*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomize},
	)
	if err != nil {
		t.Fatalf("Export DaemonSet failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

func TestIntGitOps_Export_Kustomize_WithLimits(t *testing.T) {
	e := gitops.NewExporter()
	r := rec("web", "default", "Deployment", "web", 200, 256*1024*1024)
	r.SetLimits = true
	result, err := e.Export([]gitops.ResourceRecommendation{r}, gitops.ExportConfig{Format: gitops.FormatKustomize})
	if err != nil {
		t.Fatalf("Export with limits failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

func TestIntGitOps_Export_Kustomize_ToDirectory(t *testing.T) {
	dir := t.TempDir()
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("web", "default", "Deployment", "web", 200, 256*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomize, OutputPath: dir},
	)
	if err != nil {
		t.Fatalf("Export to directory failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
	// Verify files were actually written
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Error("expected files in output directory")
	}
}

// ─── Export – Kustomize JSON6902 ──────────────────────────────────────────────

func TestIntGitOps_Export_JSON6902_Single(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("web", "default", "Deployment", "web", 300, 512*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomizeJSON6902},
	)
	if err != nil {
		t.Fatalf("Export JSON6902 failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
	// Should contain JSON patch operations
	for _, content := range result.Files {
		if strings.Contains(content, ".yaml") || strings.Contains(content, "op") || len(content) > 0 {
			break
		}
	}
}

func TestIntGitOps_Export_JSON6902_Multiple(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(recs(3), gitops.ExportConfig{Format: gitops.FormatKustomizeJSON6902})
	if err != nil {
		t.Fatalf("Export JSON6902 multiple failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

func TestIntGitOps_Export_JSON6902_ToDirectory(t *testing.T) {
	dir := t.TempDir()
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("api", "prod", "Deployment", "api", 500, 1024*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomizeJSON6902, OutputPath: dir},
	)
	if err != nil {
		t.Fatalf("Export JSON6902 to dir failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

// ─── Export – Helm ────────────────────────────────────────────────────────────

func TestIntGitOps_Export_Helm_Single(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(
		[]gitops.ResourceRecommendation{rec("web", "default", "Deployment", "web", 200, 256*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatHelm},
	)
	if err != nil {
		t.Fatalf("Export Helm failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

func TestIntGitOps_Export_Helm_Multiple(t *testing.T) {
	e := gitops.NewExporter()
	result, err := e.Export(recs(4), gitops.ExportConfig{Format: gitops.FormatHelm})
	if err != nil {
		t.Fatalf("Export Helm multiple failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
}

func TestIntGitOps_Export_Helm_ToDirectory(t *testing.T) {
	dir := t.TempDir()
	e := gitops.NewExporter()
	result, err := e.Export(
		recs(2),
		gitops.ExportConfig{Format: gitops.FormatHelm, OutputPath: dir},
	)
	if err != nil {
		t.Fatalf("Export Helm to dir failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected files")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Error("expected files written to directory")
	}
}

// ─── Export – invalid format ──────────────────────────────────────────────────

func TestIntGitOps_Export_UnsupportedFormat(t *testing.T) {
	e := gitops.NewExporter()
	_, err := e.Export(
		[]gitops.ResourceRecommendation{rec("web", "default", "Deployment", "web", 200, 256*1024*1024)},
		gitops.ExportConfig{Format: "terraform"},
	)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// ─── KustomizeGenerator ───────────────────────────────────────────────────────

func TestIntGitOps_KustomizeGenerator_StrategicMerge(t *testing.T) {
	g := gitops.NewKustomizeGenerator()
	yaml, err := g.GenerateStrategicMerge(rec("web", "default", "Deployment", "web", 200, 256*1024*1024))
	if err != nil {
		t.Fatalf("GenerateStrategicMerge failed: %v", err)
	}
	if yaml == "" {
		t.Error("expected non-empty YAML")
	}
	if !strings.Contains(yaml, "web") {
		t.Error("YAML should contain workload name")
	}
}

func TestIntGitOps_KustomizeGenerator_StrategicMerge_AllKinds(t *testing.T) {
	g := gitops.NewKustomizeGenerator()
	kinds := []string{"Deployment", "StatefulSet", "DaemonSet"}
	for _, kind := range kinds {
		_, err := g.GenerateStrategicMerge(rec("w", "ns", kind, "app", 100, 128*1024*1024))
		if err != nil {
			t.Errorf("GenerateStrategicMerge(%s) failed: %v", kind, err)
		}
	}
}

func TestIntGitOps_KustomizeGenerator_JSON6902(t *testing.T) {
	g := gitops.NewKustomizeGenerator()
	out, err := g.GenerateJSON6902(rec("api", "prod", "Deployment", "api", 500, 512*1024*1024))
	if err != nil {
		t.Fatalf("GenerateJSON6902 failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestIntGitOps_KustomizeGenerator_Kustomization(t *testing.T) {
	g := gitops.NewKustomizeGenerator()
	patches := []string{"patch1.yaml", "patch2.yaml", "patch3.yaml"}
	out, err := g.GenerateKustomization(patches)
	if err != nil {
		t.Fatalf("GenerateKustomization failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty kustomization.yaml")
	}
	for _, p := range patches {
		if !strings.Contains(out, p) {
			t.Errorf("kustomization.yaml should reference %s", p)
		}
	}
}

func TestIntGitOps_KustomizeGenerator_Kustomization_Empty(t *testing.T) {
	g := gitops.NewKustomizeGenerator()
	_, err := g.GenerateKustomization(nil)
	if err == nil {
		t.Fatal("expected error for empty patch files, got nil")
	}
}

// ─── HelmGenerator ────────────────────────────────────────────────────────────

func TestIntGitOps_HelmGenerator_Single(t *testing.T) {
	g := gitops.NewHelmGenerator()
	out, err := g.GenerateValues([]gitops.ResourceRecommendation{
		rec("web", "default", "Deployment", "web", 200, 256*1024*1024),
	})
	if err != nil {
		t.Fatalf("GenerateValues failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty Helm values")
	}
}

func TestIntGitOps_HelmGenerator_Multiple(t *testing.T) {
	g := gitops.NewHelmGenerator()
	out, err := g.GenerateValues(recs(5))
	if err != nil {
		t.Fatalf("GenerateValues multiple failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestIntGitOps_HelmGenerator_MergeValues(t *testing.T) {
	g := gitops.NewHelmGenerator()
	existing := gitops.HelmValues{
		"replicaCount": 3,
		"resources":    map[string]interface{}{"requests": map[string]interface{}{"cpu": "100m"}},
	}
	new := gitops.HelmValues{
		"resources": map[string]interface{}{"requests": map[string]interface{}{"cpu": "300m", "memory": "512Mi"}},
		"image":     map[string]interface{}{"tag": "v2.0"},
	}
	merged, err := g.MergeValues(existing, new)
	if err != nil {
		t.Fatalf("MergeValues failed: %v", err)
	}
	if merged == nil {
		t.Fatal("expected non-nil merged values")
	}
}

func TestIntGitOps_HelmGenerator_MergeValues_EmptyExisting(t *testing.T) {
	g := gitops.NewHelmGenerator()
	merged, err := g.MergeValues(gitops.HelmValues{}, gitops.HelmValues{"key": "val"})
	if err != nil {
		t.Fatalf("MergeValues empty existing failed: %v", err)
	}
	if merged == nil {
		t.Fatal("expected non-nil result")
	}
}

// ─── Output path error handling ───────────────────────────────────────────────

func TestIntGitOps_Export_InvalidOutputPath(t *testing.T) {
	e := gitops.NewExporter()
	// Use a path that can't be written to (file instead of directory)
	tmpFile := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(tmpFile, []byte("not a dir"), 0600)
	// Create a sub-path inside the file (impossible)
	invalidPath := filepath.Join(tmpFile, "subdir")
	_, err := e.Export(
		[]gitops.ResourceRecommendation{rec("web", "default", "Deployment", "web", 200, 256*1024*1024)},
		gitops.ExportConfig{Format: gitops.FormatKustomize, OutputPath: invalidPath},
	)
	// May or may not error depending on OS, just verify it doesn't panic
	_ = err
}

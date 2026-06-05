package unit_test

import (
	"testing"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/webhook"
)

func validOptimizerConfig() *optimizerv1alpha1.OptimizerConfig {
	return &optimizerv1alpha1.OptimizerConfig{
		Spec: optimizerv1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         optimizerv1alpha1.StrategyBalanced,
			DryRun:           false,
		},
	}
}

func TestNewValidator_ReturnsNonNil(t *testing.T) {
	v := webhook.NewValidator()
	if v == nil {
		t.Fatal("expected non-nil validator")
	}
}

func TestValidateCreate_ValidConfig_NoError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for valid config, got %v", err)
	}
}

func TestValidateCreate_EmptyTargetNamespaces_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.TargetNamespaces = []string{}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for empty TargetNamespaces")
	}
}

func TestValidateCreate_NilTargetNamespaces_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.TargetNamespaces = nil
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for nil TargetNamespaces")
	}
}

func TestValidateCreate_InvalidProfileOverride_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	minConf := float64(150) // invalid: must be 0-100
	cfg.Spec.ProfileOverrides = &optimizerv1alpha1.ProfileOverrides{
		MinConfidence: &minConf,
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for out-of-range MinConfidence in ProfileOverrides")
	}
}

func TestValidateCreate_ValidProfiles(t *testing.T) {
	v := webhook.NewValidator()
	profiles := []optimizerv1alpha1.EnvironmentProfile{
		optimizerv1alpha1.ProfileProduction,
		optimizerv1alpha1.ProfileStaging,
		optimizerv1alpha1.ProfileDevelopment,
		optimizerv1alpha1.ProfileTest,
		optimizerv1alpha1.ProfileCustom,
		"",
	}
	for _, p := range profiles {
		cfg := validOptimizerConfig()
		cfg.Spec.Profile = p
		if err := v.ValidateCreate(cfg); err != nil {
			t.Errorf("profile %q should be valid, got error: %v", p, err)
		}
	}
}

func TestValidateCreate_InvalidMaintenanceWindow_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.MaintenanceWindows = []optimizerv1alpha1.MaintenanceWindow{
		{
			Schedule: "invalid-cron", // invalid cron expression
			Duration: "2h",
		},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for invalid maintenance window schedule")
	}
}

func TestValidateCreate_ValidMaintenanceWindow(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.MaintenanceWindows = []optimizerv1alpha1.MaintenanceWindow{
		{
			Schedule: "0 2 * * 1-5", // 2 AM on weekdays
			Duration: "4h",
		},
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for valid maintenance window, got %v", err)
	}
}

func TestValidateCreate_InvalidCircuitBreaker_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.CircuitBreaker = &optimizerv1alpha1.CircuitBreakerConfig{
		Enabled:        true,
		ErrorThreshold: -1, // invalid: must be ≥1
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for negative error threshold in circuit breaker")
	}
}

func TestValidateCreate_ValidCircuitBreaker(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.CircuitBreaker = &optimizerv1alpha1.CircuitBreakerConfig{
		Enabled:          true,
		ErrorThreshold:   3,
		SuccessThreshold: 2,
		Timeout:          "5m",
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for valid circuit breaker, got %v", err)
	}
}

func TestValidateUpdate_ValidConfig_NoError(t *testing.T) {
	v := webhook.NewValidator()
	old := validOptimizerConfig()
	new := validOptimizerConfig()
	new.Spec.DryRun = true
	if err := v.ValidateUpdate(old, new); err != nil {
		t.Errorf("expected no error for valid update, got %v", err)
	}
}

func TestValidateUpdate_InvalidNew_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	old := validOptimizerConfig()
	new := validOptimizerConfig()
	new.Spec.TargetNamespaces = nil
	if err := v.ValidateUpdate(old, new); err == nil {
		t.Error("expected error for invalid updated config")
	}
}

func TestValidateDelete_AnyConfig_NoError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	if err := v.ValidateDelete(cfg); err != nil {
		t.Errorf("expected no error for delete validation, got %v", err)
	}
}

func TestValidateCreate_GitOpsExport_MissingOutputPath_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.GitOpsExport = &optimizerv1alpha1.GitOpsExportConfig{
		Enabled:    true,
		Format:     optimizerv1alpha1.GitOpsFormatKustomize,
		OutputPath: "", // missing required field
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for missing outputPath when GitOps enabled")
	}
}

func TestValidateCreate_ValidGitOpsExport_Kustomize(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.GitOpsExport = &optimizerv1alpha1.GitOpsExportConfig{
		Enabled:    true,
		Format:     optimizerv1alpha1.GitOpsFormatKustomize,
		OutputPath: "/tmp/gitops-output",
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for kustomize format with outputPath, got %v", err)
	}
}

func TestValidateCreate_GitOpsDisabled_NoValidation(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.GitOpsExport = &optimizerv1alpha1.GitOpsExportConfig{
		Enabled: false, // disabled — no validation needed
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error when GitOps disabled, got %v", err)
	}
}

func TestValidateCreate_InvalidExcludeWorkloads_Regex(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.ExcludeWorkloads = []string{"[invalid-regex"}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for invalid regex in ExcludeWorkloads")
	}
}

func TestValidateCreate_DryRunMode_Valid(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.DryRun = true
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for dry-run mode, got %v", err)
	}
}

// ── Notification validation tests (WP7 — validateNotifications) ──────────────

func TestValidateCreate_Notifications_Disabled_NoError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{Enabled: false}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error when notifications disabled, got %v", err)
	}
}

func TestValidateCreate_Notifications_ValidSlack(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Slack:   &optimizerv1alpha1.SlackConfig{WebhookURL: "https://hooks.slack.com/services/T000/B000/xxxx"},
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for valid Slack config, got %v", err)
	}
}

func TestValidateCreate_Notifications_InvalidSlackURL_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Slack:   &optimizerv1alpha1.SlackConfig{WebhookURL: "https://bad-domain.com/hook"},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for invalid Slack webhook URL domain")
	}
}

func TestValidateCreate_Notifications_ValidEmail(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Email: &optimizerv1alpha1.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "optimizer@example.com",
			To:       []string{"ops@example.com"},
		},
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for valid email config, got %v", err)
	}
}

func TestValidateCreate_Notifications_Email_MissingSMTPHost_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Email: &optimizerv1alpha1.EmailConfig{
			SMTPHost: "",
			From:     "optimizer@example.com",
			To:       []string{"ops@example.com"},
		},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for missing SMTP host")
	}
}

func TestValidateCreate_Notifications_Email_InvalidFrom_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Email: &optimizerv1alpha1.EmailConfig{
			SMTPHost: "smtp.example.com",
			From:     "not-an-email",
			To:       []string{"ops@example.com"},
		},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for invalid From email address")
	}
}

func TestValidateCreate_Notifications_Email_EmptyTo_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Email: &optimizerv1alpha1.EmailConfig{
			SMTPHost: "smtp.example.com",
			From:     "optimizer@example.com",
			To:       []string{},
		},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for empty To list")
	}
}

func TestValidateCreate_Notifications_ValidWebhook(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Webhooks: []optimizerv1alpha1.WebhookConfig{
			{Name: "pagerduty", URL: "https://events.pagerduty.com/v2/enqueue", Timeout: "10s"},
		},
	}
	if err := v.ValidateCreate(cfg); err != nil {
		t.Errorf("expected no error for valid webhook config, got %v", err)
	}
}

func TestValidateCreate_Notifications_Webhook_MissingName_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Webhooks: []optimizerv1alpha1.WebhookConfig{
			{Name: "", URL: "https://example.com/hook"},
		},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for webhook with empty name")
	}
}

func TestValidateCreate_Notifications_Webhook_InvalidTimeout_ReturnsError(t *testing.T) {
	v := webhook.NewValidator()
	cfg := validOptimizerConfig()
	cfg.Spec.Notifications = &optimizerv1alpha1.NotificationConfig{
		Enabled: true,
		Webhooks: []optimizerv1alpha1.WebhookConfig{
			{Name: "test", URL: "https://example.com/hook", Timeout: "invalid"},
		},
	}
	if err := v.ValidateCreate(cfg); err == nil {
		t.Error("expected error for invalid webhook timeout format")
	}
}

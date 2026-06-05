package unit_test

import (
	"context"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/notifications"
)

func makeEvent(typ notifications.EventType, sev notifications.Severity) *notifications.Event {
	return &notifications.Event{
		Type:      typ,
		Severity:  sev,
		Namespace: "default",
		Workload:  "stress-cyclic",
		Message:   "test notification",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"key": "value"},
	}
}

// ── Event helpers ─────────────────────────────────────────────────────────────

func TestEvent_GetSeverityColor(t *testing.T) {
	cases := []struct {
		sev   notifications.Severity
		color string
	}{
		{notifications.SeverityInfo, "#36a64f"},
		{notifications.SeverityWarning, "#ff9900"},
		{notifications.SeverityCritical, "#ff0000"},
		{"unknown", "#808080"},
	}
	for _, tc := range cases {
		e := &notifications.Event{Severity: tc.sev}
		if got := e.GetSeverityColor(); got != tc.color {
			t.Errorf("sev %s: expected color %s, got %s", tc.sev, tc.color, got)
		}
	}
}

func TestEvent_GetTitle(t *testing.T) {
	e := &notifications.Event{
		Type:     notifications.EventOOMDetected,
		Severity: notifications.SeverityCritical,
	}
	title := e.GetTitle()
	if title == "" {
		t.Error("expected non-empty title")
	}
	if title != "OOMDetected - critical" {
		t.Errorf("unexpected title: %q", title)
	}
}

// ── SlackNotifier ─────────────────────────────────────────────────────────────

func TestSlackNotifier_Name(t *testing.T) {
	s := notifications.NewSlackNotifier("https://hooks.slack.com/services/test")
	if s.Name() == "" {
		t.Error("expected non-empty name")
	}
}

func TestSlackNotifier_Send_InvalidURL_ReturnsError(t *testing.T) {
	s := notifications.NewSlackNotifier("http://localhost:19999/nosuchendpoint")
	err := s.Send(context.Background(), makeEvent(notifications.EventOOMDetected, notifications.SeverityCritical))
	if err == nil {
		t.Error("expected error for invalid Slack URL")
	}
}

func TestSlackNotifier_Send_ContextCancelled(t *testing.T) {
	s := notifications.NewSlackNotifier("http://localhost:19999/nosuchendpoint")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Send(ctx, makeEvent(notifications.EventSLAViolation, notifications.SeverityWarning))
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// ── WebhookNotifier ───────────────────────────────────────────────────────────

func TestWebhookNotifier_Name(t *testing.T) {
	w := notifications.NewWebhookNotifier(notifications.WebhookConfig{
		URL: "https://example.com/hook",
	})
	if w.Name() == "" {
		t.Error("expected non-empty name")
	}
}

func TestWebhookNotifier_Send_InvalidURL_ReturnsError(t *testing.T) {
	w := notifications.NewWebhookNotifier(notifications.WebhookConfig{
		URL:     "http://localhost:19998/nosuchpath",
		Timeout: 200 * time.Millisecond,
	})
	err := w.Send(context.Background(), makeEvent(notifications.EventRollbackExecuted, notifications.SeverityInfo))
	if err == nil {
		t.Error("expected error for unreachable webhook URL")
	}
}

func TestWebhookNotifier_Send_WithHeaders(t *testing.T) {
	w := notifications.NewWebhookNotifier(notifications.WebhookConfig{
		URL:     "http://localhost:19997/nosuch",
		Timeout: 200 * time.Millisecond,
		Headers: map[string]string{"X-Token": "secret"},
	})
	err := w.Send(context.Background(), makeEvent(notifications.EventMemoryLeakDetected, notifications.SeverityWarning))
	if err == nil {
		t.Error("expected error for unreachable webhook URL")
	}
}

// ── EmailNotifier ─────────────────────────────────────────────────────────────

func TestEmailNotifier_Name(t *testing.T) {
	e := notifications.NewEmailNotifier(notifications.EmailConfig{
		From:     "optimizer@example.com",
		To:       []string{"ops@example.com"},
		SMTPHost: "localhost",
		SMTPPort: 25,
	})
	if e.Name() == "" {
		t.Error("expected non-empty name")
	}
}

func TestEmailNotifier_Send_InvalidSMTP_ReturnsError(t *testing.T) {
	e := notifications.NewEmailNotifier(notifications.EmailConfig{
		From:     "optimizer@example.com",
		To:       []string{"ops@example.com"},
		SMTPHost: "localhost",
		SMTPPort: 19996,
	})
	err := e.Send(context.Background(), makeEvent(notifications.EventCircuitBreakerTriggered, notifications.SeverityCritical))
	if err == nil {
		t.Error("expected error for unreachable SMTP server")
	}
}

func TestEmailNotifier_Send_TLS_InvalidSMTP_ReturnsError(t *testing.T) {
	e := notifications.NewEmailNotifier(notifications.EmailConfig{
		From:     "optimizer@example.com",
		To:       []string{"ops@example.com"},
		SMTPHost: "localhost",
		SMTPPort: 19995,
		UseTLS:   true,
	})
	err := e.Send(context.Background(), makeEvent(notifications.EventSLAViolation, notifications.SeverityWarning))
	if err == nil {
		t.Error("expected error for unreachable TLS SMTP server")
	}
}

func TestEmailNotifier_Send_MultipleRecipients(t *testing.T) {
	e := notifications.NewEmailNotifier(notifications.EmailConfig{
		From:     "optimizer@example.com",
		To:       []string{"ops@example.com", "devops@example.com"},
		SMTPHost: "localhost",
		SMTPPort: 19994,
	})
	err := e.Send(context.Background(), makeEvent(notifications.EventRecommendationApplied, notifications.SeverityInfo))
	if err == nil {
		t.Error("expected error for unreachable SMTP server")
	}
}

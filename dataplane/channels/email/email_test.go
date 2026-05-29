package email

import (
	"net/smtp"
	"strings"
	"testing"
)

func TestParseInbound_PlainText(t *testing.T) {
	raw := []byte("From: alice@example.com\r\n" +
		"To: bot@ai-keeper.io\r\n" +
		"Subject: Contract Review Request\r\n" +
		"Message-ID: <msg001@example.com>\r\n" +
		"In-Reply-To: <prev@example.com>\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"\r\n" +
		"Please review the attached contract.\r\n")

	adapter := NewEmailAdapter()
	email, err := adapter.ParseInbound(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if email.From != "alice@example.com" {
		t.Errorf("expected From alice@example.com, got %s", email.From)
	}
	if email.To != "bot@ai-keeper.io" {
		t.Errorf("expected To bot@ai-keeper.io, got %s", email.To)
	}
	if email.Subject != "Contract Review Request" {
		t.Errorf("expected Subject 'Contract Review Request', got %s", email.Subject)
	}
	if email.MessageID != "<msg001@example.com>" {
		t.Errorf("expected MessageID <msg001@example.com>, got %s", email.MessageID)
	}
	if email.InReplyTo != "<prev@example.com>" {
		t.Errorf("expected InReplyTo <prev@example.com>, got %s", email.InReplyTo)
	}
	if !strings.Contains(email.Body, "Please review the attached contract.") {
		t.Errorf("expected body to contain message text, got %s", email.Body)
	}
}

func TestParseInbound_Multipart(t *testing.T) {
	raw := []byte("From: bob@example.com\r\n" +
		"To: agent@ai-keeper.io\r\n" +
		"Subject: Multi-part Test\r\n" +
		"Message-ID: <msg002@example.com>\r\n" +
		"Content-Type: multipart/alternative; boundary=\"boundary123\"\r\n" +
		"\r\n" +
		"--boundary123\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"\r\n" +
		"This is the plain text part.\r\n" +
		"--boundary123\r\n" +
		"Content-Type: text/html; charset=\"utf-8\"\r\n" +
		"\r\n" +
		"<p>This is the HTML part.</p>\r\n" +
		"--boundary123--\r\n")

	adapter := NewEmailAdapter()
	email, err := adapter.ParseInbound(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if email.From != "bob@example.com" {
		t.Errorf("expected From bob@example.com, got %s", email.From)
	}
	if !strings.Contains(email.Body, "This is the plain text part.") {
		t.Errorf("expected plain text body, got %s", email.Body)
	}
}

func TestParseInbound_NoContentType(t *testing.T) {
	raw := []byte("From: charlie@example.com\r\n" +
		"To: bot@ai-keeper.io\r\n" +
		"Subject: Simple Email\r\n" +
		"\r\n" +
		"Just a simple message.\r\n")

	adapter := NewEmailAdapter()
	email, err := adapter.ParseInbound(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(email.Body, "Just a simple message.") {
		t.Errorf("expected body text, got %s", email.Body)
	}
}

func TestParseInbound_EmptyInput(t *testing.T) {
	adapter := NewEmailAdapter()
	_, err := adapter.ParseInbound(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}

	_, err = adapter.ParseInbound([]byte{})
	if err == nil {
		t.Fatal("expected error for empty byte slice")
	}
}

func TestParseInbound_InvalidMessage(t *testing.T) {
	adapter := NewEmailAdapter()
	_, err := adapter.ParseInbound([]byte("not a valid email"))
	// mail.ReadMessage requires headers, so garbage input without \r\n\r\n
	// may either parse with empty body or fail — we just verify no panic.
	_ = err
}

func TestSendReply(t *testing.T) {
	var sentTo []string
	var sentMsg []byte

	adapter := NewEmailAdapter(
		WithSMTPConfig(SMTPConfig{
			Host: "smtp.example.com",
			Port: 587,
			From: "bot@ai-keeper.io",
		}),
	)
	// Inject test sender.
	adapter.sendFunc = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		sentTo = to
		sentMsg = msg
		return nil
	}

	err := adapter.SendReply("<orig-msg-id@example.com>", "alice@example.com", "Re: Contract Review", "Looks good!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sentTo) != 1 || sentTo[0] != "alice@example.com" {
		t.Errorf("expected recipient alice@example.com, got %v", sentTo)
	}

	msgStr := string(sentMsg)
	if !strings.Contains(msgStr, "From: bot@ai-keeper.io") {
		t.Error("expected From header in sent message")
	}
	if !strings.Contains(msgStr, "To: alice@example.com") {
		t.Error("expected To header in sent message")
	}
	if !strings.Contains(msgStr, "Subject: Re: Contract Review") {
		t.Error("expected Subject header in sent message")
	}
	if !strings.Contains(msgStr, "In-Reply-To: <orig-msg-id@example.com>") {
		t.Error("expected In-Reply-To header in sent message")
	}
	if !strings.Contains(msgStr, "Looks good!") {
		t.Error("expected body in sent message")
	}
}

func TestSendReply_NoContext(t *testing.T) {
	var sentMsg []byte

	adapter := NewEmailAdapter(
		WithSMTPConfig(SMTPConfig{
			Host: "smtp.example.com",
			Port: 587,
			From: "bot@ai-keeper.io",
		}),
	)
	adapter.sendFunc = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		sentMsg = msg
		return nil
	}

	err := adapter.SendReply("", "user@example.com", "Hello", "Welcome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgStr := string(sentMsg)
	if strings.Contains(msgStr, "In-Reply-To:") {
		t.Error("expected no In-Reply-To header when context is empty")
	}
}

func TestSendReply_Validation(t *testing.T) {
	adapter := NewEmailAdapter(
		WithSMTPConfig(SMTPConfig{
			Host: "smtp.example.com",
			Port: 587,
			From: "bot@ai-keeper.io",
		}),
	)
	adapter.sendFunc = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		return nil
	}

	tests := []struct {
		name    string
		to      string
		subject string
		body    string
		wantErr bool
	}{
		{"empty to", "", "subject", "body", true},
		{"empty subject", "to@example.com", "", "body", true},
		{"empty body", "to@example.com", "subject", "", true},
		{"valid", "to@example.com", "subject", "body", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.SendReply("", tt.to, tt.subject, tt.body)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSendReply_NoFromConfigured(t *testing.T) {
	adapter := NewEmailAdapter() // No SMTP config.
	adapter.sendFunc = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		return nil
	}

	err := adapter.SendReply("", "to@example.com", "subject", "body")
	if err == nil {
		t.Fatal("expected error when From is not configured")
	}
}

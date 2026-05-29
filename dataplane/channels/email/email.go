// Package email implements the Email channel adapter for AIP Gateway.
// It handles SMTP inbound email parsing (MIME) and outbound reply sending.
package email

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/smtp"
	"strings"
)

// InboundEmail represents a parsed incoming email message.
type InboundEmail struct {
	From      string
	To        string
	Subject   string
	Body      string
	MessageID string
	InReplyTo string
}

// SMTPConfig holds SMTP server configuration for sending emails.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // Default sender address.
}

// EmailAdapter handles inbound email parsing and outbound SMTP delivery.
type EmailAdapter struct {
	config SMTPConfig

	// sendFunc allows injection of a custom sender for testing.
	sendFunc func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

// Option configures an EmailAdapter.
type Option func(*EmailAdapter)

// WithSMTPConfig sets the SMTP configuration.
func WithSMTPConfig(cfg SMTPConfig) Option {
	return func(a *EmailAdapter) { a.config = cfg }
}

// NewEmailAdapter creates a new Email channel adapter.
func NewEmailAdapter(opts ...Option) *EmailAdapter {
	a := &EmailAdapter{
		sendFunc: smtp.SendMail,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ParseInbound parses a raw MIME email into an InboundEmail struct.
func (a *EmailAdapter) ParseInbound(raw []byte) (*InboundEmail, error) {
	if len(raw) == 0 {
		return nil, errors.New("email: raw message is empty")
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("email: failed to parse message: %w", err)
	}

	from := msg.Header.Get("From")
	to := msg.Header.Get("To")
	subject := msg.Header.Get("Subject")
	messageID := msg.Header.Get("Message-ID")
	inReplyTo := msg.Header.Get("In-Reply-To")

	body, err := extractBody(msg)
	if err != nil {
		return nil, fmt.Errorf("email: failed to extract body: %w", err)
	}

	return &InboundEmail{
		From:      from,
		To:        to,
		Subject:   subject,
		Body:      body,
		MessageID: messageID,
		InReplyTo: inReplyTo,
	}, nil
}

// SendReply sends an email reply via SMTP.
func (a *EmailAdapter) SendReply(ctx, to, subject, body string) error {
	if to == "" {
		return errors.New("email: recipient address is required")
	}
	if subject == "" {
		return errors.New("email: subject is required")
	}
	if body == "" {
		return errors.New("email: body is required")
	}

	from := a.config.From
	if from == "" {
		return errors.New("email: sender address not configured")
	}

	// Build the email message.
	var msg bytes.Buffer
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	if ctx != "" {
		msg.WriteString("In-Reply-To: " + ctx + "\r\n")
		msg.WriteString("References: " + ctx + "\r\n")
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := fmt.Sprintf("%s:%d", a.config.Host, a.config.Port)

	var auth smtp.Auth
	if a.config.Username != "" {
		auth = smtp.PlainAuth("", a.config.Username, a.config.Password, a.config.Host)
	}

	return a.sendFunc(addr, auth, from, []string{to}, msg.Bytes())
}

// extractBody extracts the plain text body from an email message.
// It handles both plain text messages and multipart MIME messages.
func extractBody(msg *mail.Message) (string, error) {
	contentType := msg.Header.Get("Content-Type")

	// Default to plain text if no content type specified.
	if contentType == "" {
		b, err := io.ReadAll(msg.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// If we can't parse content type, try reading body directly.
		b, err := io.ReadAll(msg.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// Simple text content.
	if strings.HasPrefix(mediaType, "text/") {
		b, err := io.ReadAll(msg.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// Multipart content — find the text/plain part.
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "", errors.New("email: multipart boundary not found")
		}

		reader := multipart.NewReader(msg.Body, boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}

			partContentType := part.Header.Get("Content-Type")
			partMedia, _, _ := mime.ParseMediaType(partContentType)

			if partMedia == "text/plain" || partMedia == "" {
				b, err := io.ReadAll(part)
				if err != nil {
					return "", err
				}
				return string(b), nil
			}
		}

		return "", errors.New("email: no text/plain part found in multipart message")
	}

	// Fallback: read body directly.
	b, err := io.ReadAll(msg.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

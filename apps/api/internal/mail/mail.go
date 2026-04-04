package mail

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// Mailer sends transactional emails.
type Mailer interface {
	SendInvite(ctx context.Context, to, inviteURL, tenantName, inviterName string) error
}

// NoopMailer logs the invite URL without sending — use in development.
type NoopMailer struct{}

func (NoopMailer) SendInvite(_ context.Context, to, inviteURL, tenantName, inviterName string) error {
	fmt.Printf("[mail] invite to=%s tenant=%q inviter=%q url=%s\n", to, tenantName, inviterName, inviteURL)
	return nil
}

// SMTPMailer sends email via SMTP (Mailtrap, SES relay, Postmark SMTP, etc.).
type SMTPMailer struct {
	Host     string // e.g. "smtp.mailtrap.io"
	Port     string // e.g. "587"
	User     string
	Password string
	From     string
}

func (m *SMTPMailer) SendInvite(_ context.Context, to, inviteURL, tenantName, inviterName string) error {
	auth := smtp.PlainAuth("", m.User, m.Password, m.Host)
	body := strings.Join([]string{
		"From: " + m.From,
		"To: " + to,
		"Subject: You've been invited to " + tenantName,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		inviterName + " has invited you to join " + tenantName + ".",
		"",
		"Accept your invitation:",
		inviteURL,
		"",
		"This link expires in 48 hours.",
	}, "\r\n")
	return smtp.SendMail(m.Host+":"+m.Port, auth, m.From, []string{to}, []byte(body))
}

package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

type Mailer struct {
	host string
	port string
	from string
	user string
	pass string
}

func New(host, port, user, pass, from string) *Mailer {
	return &Mailer{host: host, port: port, from: from, user: user, pass: pass}
}

func (m *Mailer) Send(to, subject, body string) error {
	addr := m.host + ":" + m.port

	msg := strings.Join([]string{
		"From: French 75 Tracker <" + m.from + ">",
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")

	// Use auth only if credentials are provided (localhost Postfix needs none)
	var auth smtp.Auth
	if m.user != "" && m.pass != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}

	return smtp.SendMail(addr, auth, m.from, []string{to}, []byte(msg))
}

func (m *Mailer) SendVerification(to, name, link string) error {
	body := fmt.Sprintf(`Hi %s,

Please verify your email address to request access to French 75 Tracker.

Click this link (valid for 24 hours):
%s

If you did not request this, ignore this email.

— French 75 Tracker`, name, link)

	return m.Send(to, "Verify your email — French 75 Tracker", body)
}

func (m *Mailer) SendWelcome(to, name, link string) error {
	body := fmt.Sprintf(`Hi %s,

Your request to join French 75 Tracker has been approved.

Set up your passkey (Face ID, Touch ID, or security key) using this link:
%s

This link is valid for 48 hours. After setting up your passkey you can log in
at any time using your device.

Welcome aboard,
— French 75 Tracker`, name, link)

	return m.Send(to, "Welcome to French 75 Tracker — set up your passkey", body)
}

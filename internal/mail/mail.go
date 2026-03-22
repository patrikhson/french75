package mail

import (
	"crypto/tls"
	"fmt"
	"net"
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

	// Dial manually instead of using smtp.SendMail, which passes the dial
	// address as the TLS ServerName. Connecting to "localhost" while the
	// server's cert is for another hostname ("mail.paftech.se") causes a
	// cert verification failure. For localhost we skip cert verification
	// (loopback traffic; TLS adds no security value here).
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		isLocal := m.host == "localhost" || m.host == "127.0.0.1"
		if err := c.StartTLS(&tls.Config{
			ServerName:         m.host,
			InsecureSkipVerify: isLocal,
		}); err != nil {
			return err
		}
	}

	if m.user != "" && m.pass != "" {
		if err := c.Auth(smtp.PlainAuth("", m.user, m.pass, m.host)); err != nil {
			return err
		}
	}

	msg := strings.Join([]string{
		"From: French 75 Tracker <" + m.from + ">",
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")

	if err := c.Mail(m.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
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

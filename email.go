package main

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

func sendWelcomeEmailOnce(db *sql.DB, toEmail, company string) {
	if toEmail == "" {
		return
	}
	subject := "Welcome to statuspage.app.nz"
	if !reserveEmail(db, toEmail, "welcome", subject) {
		return
	}
	body := welcomeEmailBody(company)
	if err := sendEmail(toEmail, subject, body); err != nil {
		log.Printf("welcome email failed: %v", err)
	}
}

func reserveEmail(db *sql.DB, toEmail, kind, subject string) bool {
	if db == nil {
		return true
	}
	var inserted bool
	err := db.QueryRow(`
		INSERT INTO email_events (recipient, kind, subject)
		VALUES ($1, $2, $3)
		ON CONFLICT (recipient, kind) DO NOTHING
		RETURNING true
	`, strings.ToLower(strings.TrimSpace(toEmail)), kind, subject).Scan(&inserted)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		log.Printf("email event reserve failed: %v", err)
		return false
	}
	return inserted
}

func welcomeEmailBody(company string) string {
	name := htmlEscape(company)
	return fmt.Sprintf(`
<h1>Welcome to statuspage.app.nz</h1>
<p>We received the signup for %s. The fastest useful setup is three public checks: your website, your login or dashboard, and the API or checkout path customers depend on.</p>
<p>After that, add the communication pieces that reduce support load during incidents:</p>
<ul>
  <li>a plain-language status page name and description,</li>
  <li>a customer-facing domain such as status.yourcompany.co.nz or your statuspage.app.nz subdomain,</li>
  <li>one clear contact email for alerts and updates,</li>
  <li>a short incident template: what is affected, what still works, and when the next update will land.</li>
</ul>
<p>Annual billing is the default at $190/year. Monthly billing is available at $19/month from the account page.</p>
<p>Manage your account here: <a href="https://statuspage.app.nz/account">https://statuspage.app.nz/account</a></p>
`, name)
}

func sendEmail(toEmail, subject, htmlBody string) error {
	region := getenv("AWS_REGION", "us-east-1")
	smtpUser := os.Getenv("AWS_SMTP_USERNAME")
	smtpPass := os.Getenv("AWS_SMTP_PASSWORD")
	if smtpUser == "" || smtpPass == "" {
		return fmt.Errorf("AWS_SMTP_USERNAME and AWS_SMTP_PASSWORD required")
	}

	smtpHost := fmt.Sprintf("email-smtp.%s.amazonaws.com", region)
	fromEmail := getenv("SES_FROM_EMAIL", "lee.penkman@netwrck.com")
	fromName := getenv("SES_FROM_NAME", "statuspage.app.nz")
	boundary := "----=_Part_" + randomHex(8)
	plainText := plainTextFromHTML(htmlBody)

	msg := fmt.Sprintf(
		"From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n--%s--\r\n",
		fromName, fromEmail, toEmail, subject, boundary, boundary, plainText, boundary, htmlBody, boundary,
	)

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	tlsConfig := &tls.Config{ServerName: smtpHost}
	conn, err := tls.Dial("tcp", smtpHost+":465", tlsConfig)
	if err != nil {
		if err := smtp.SendMail(smtpHost+":587", auth, fromEmail, []string{toEmail}, []byte(msg)); err != nil {
			return fmt.Errorf("SMTP send failed: %w", err)
		}
		return nil
	}

	c, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %w", err)
	}
	defer c.Close()
	if err = c.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}
	if err = c.Mail(fromEmail); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err = c.Rcpt(toEmail); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err = w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("SMTP close failed: %w", err)
	}
	return c.Quit()
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

func plainTextFromHTML(s string) string {
	r := strings.NewReplacer(
		"<h1>", "", "</h1>", "\n\n",
		"<p>", "", "</p>", "\n\n",
		"<ul>", "", "</ul>", "\n",
		"<li>", "- ", "</li>", "\n",
		"<strong>", "", "</strong>", "",
		"<a href=\"https://statuspage.app.nz/account\">", "", "</a>", "",
	)
	return strings.TrimSpace(r.Replace(s))
}

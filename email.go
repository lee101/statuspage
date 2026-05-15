package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

func sendWelcomeEmail(toEmail, company string) {
	if toEmail == "" {
		return
	}
	body := fmt.Sprintf(`<p>Welcome to statuspage.app.nz.</p><p>We received the signup for %s and will help you configure monitors, branding, alerts, and your customer directory backlink.</p>`, htmlEscape(company))
	if err := sendEmail(toEmail, "Welcome to statuspage.app.nz", body); err != nil {
		log.Printf("welcome email failed: %v", err)
	}
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
	plainText := "Welcome to statuspage.app.nz."

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

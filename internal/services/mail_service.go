package services

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

type MailService struct {
	host, port, from   string
	username, password string
	enabled            bool
}

func NewMailServiceFromEnv() *MailService {
	appEnv, host, port, from := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))), strings.TrimSpace(os.Getenv("SMTP_HOST")), strings.TrimSpace(os.Getenv("SMTP_PORT")), strings.TrimSpace(os.Getenv("SMTP_FROM"))
	username, password := strings.TrimSpace(os.Getenv("SMTP_USERNAME")), os.Getenv("SMTP_PASSWORD")
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "1025"
	}
	if from == "" {
		from = "no-reply@ecommerce.local"
	}
	enabled := host != "" && port != "" && from != "" && (appEnv != "production" || (username != "" && password != ""))
	return &MailService{host: host, port: port, from: from, username: username, password: password, enabled: enabled}
}

func (s *MailService) SendOrderCreated(user models.User, order models.Order) {
	_ = s.send(user.Email, fmt.Sprintf("Order #%d has been created", order.ID), fmt.Sprintf("Hello %s,\n\nyour order #%d has been created successfully.\nStatus: %s\nTotal: %d.%02d EUR\n\nThank you.\n", user.Name, order.ID, order.Status, order.TotalCents/100, order.TotalCents%100))
}

func (s *MailService) SendPasswordChanged(user models.User) {
	_ = s.send(user.Email, "Your password has been changed", fmt.Sprintf("Hello %s,\n\nyour password has been changed successfully.\n", user.Name))
}

func (s *MailService) SendOrderStatusChanged(user models.User, order models.Order) {
	_ = s.send(user.Email, fmt.Sprintf("Order #%d: status updated", order.ID), fmt.Sprintf("Hello %s,\n\nthe status of your order #%d is now: %s.\n", user.Name, order.ID, order.Status))
}

func (s *MailService) SendSecurityCode(to, displayName, code string, expiresIn time.Duration) error {
	body := fmt.Sprintf("Hello %s,\n\nYour NordShop email verification code is: %s\n\nIt expires in %d minutes. If you did not request this change, ignore this message.\n", displayName, code, int(expiresIn.Minutes()))
	htmlBody := fmt.Sprintf("<!doctype html><html><body style=\"font-family:Arial,sans-serif;color:#172033\"><div style=\"max-width:560px;margin:auto;padding:32px\"><h1 style=\"font-size:24px\">Confirm your email address</h1><p>Hello %s,</p><p>Use this security code to confirm your new NordShop email address:</p><p style=\"font:700 30px monospace;letter-spacing:4px;background:#eef2ff;padding:18px;border-radius:12px;text-align:center\">%s</p><p>This code expires in %d minutes. If you did not request this change, you can safely ignore this message.</p></div></body></html>", html.EscapeString(displayName), html.EscapeString(code), int(expiresIn.Minutes()))
	return s.sendAlternative(to, "Confirm your NordShop email address", body, htmlBody)
}

func (s *MailService) sendAlternative(to, subject, textBody, htmlBody string) error {
	if !s.enabled || to == "" {
		return errors.New("email delivery is not configured")
	}
	if strings.ContainsAny(to, "\r\n") {
		return errors.New("invalid email recipient")
	}
	const boundary = "nordshop-security-message"
	message := "To: " + to + "\r\nFrom: " + s.from + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n" +
		"--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + textBody + "\r\n" +
		"--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + htmlBody + "\r\n--" + boundary + "--\r\n"
	return s.deliver(to, message)
}

func (s *MailService) send(to, subject, body string) error {
	if !s.enabled || to == "" {
		return errors.New("email delivery is not configured")
	}
	if strings.ContainsAny(to, "\r\n") {
		return errors.New("invalid email recipient")
	}
	message := "To: " + to + "\r\nFrom: " + s.from + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	return s.deliver(to, message)
}

func (s *MailService) deliver(to, message string) error {
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	if err := smtp.SendMail(s.host+":"+s.port, auth, s.from, []string{to}, []byte(message)); err != nil {
		errorMessage := strings.NewReplacer(to, "[recipient]", s.from, "[sender]").Replace(err.Error())
		slog.Warn("informational email delivery failed", "error", errorMessage)
		return errors.New("email delivery failed")
	}
	return nil
}

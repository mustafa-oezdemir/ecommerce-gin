package services

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

type MailService struct {
	host, port, from string
	enabled          bool
}

func NewMailServiceFromEnv() *MailService {
	appEnv, host, port, from := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))), strings.TrimSpace(os.Getenv("SMTP_HOST")), strings.TrimSpace(os.Getenv("SMTP_PORT")), strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "1025"
	}
	if from == "" {
		from = "no-reply@ecommerce.local"
	}
	return &MailService{host: host, port: port, from: from, enabled: appEnv != "production"}
}

func (s *MailService) SendOrderCreated(user models.User, order models.Order) {
	s.send(user.Email, fmt.Sprintf("Bestellung #%d wurde erstellt", order.ID), fmt.Sprintf("Hallo %s,\n\ndeine Bestellung #%d wurde erfolgreich erstellt.\nStatus: %s\nGesamtbetrag: %d,%02d EUR\n\nVielen Dank.\n", user.Name, order.ID, order.Status, order.TotalCents/100, order.TotalCents%100))
}

func (s *MailService) SendPasswordChanged(user models.User) {
	s.send(user.Email, "Passwort wurde geändert", fmt.Sprintf("Hallo %s,\n\ndein Passwort wurde erfolgreich geändert.\n", user.Name))
}

func (s *MailService) send(to, subject, body string) {
	if !s.enabled || to == "" {
		return
	}
	message := "To: " + to + "\r\nFrom: " + s.from + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	if err := smtp.SendMail(s.host+":"+s.port, nil, s.from, []string{to}, []byte(message)); err != nil {
		log.Printf("informational email delivery failed: %v", err)
	}
}

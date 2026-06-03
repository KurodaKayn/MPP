package email

import (
	"fmt"
	"net/smtp"
)

type EmailService interface {
	SendVerificationCode(to, code string) error
	SendPasswordResetCode(to, code string) error
}

type SMTPEmailService struct {
	Host     string
	Port     int
	From     string
	Password string
}

func NewSMTPEmailService(host string, port int, from, password string) *SMTPEmailService {
	return &SMTPEmailService{
		Host:     host,
		Port:     port,
		From:     from,
		Password: password,
	}
}

func (s *SMTPEmailService) SendVerificationCode(to, code string) error {
	subject := "Verification Code"
	body := fmt.Sprintf("Your verification code is: %s", code)
	return s.send(to, subject, body)
}

func (s *SMTPEmailService) SendPasswordResetCode(to, code string) error {
	subject := "Password Reset Code"
	body := fmt.Sprintf("Your password reset code is: %s", code)
	return s.send(to, subject, body)
}

func (s *SMTPEmailService) send(to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", s.From, to, subject, body)
	auth := smtp.PlainAuth("", s.From, s.Password, s.Host)
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	return smtp.SendMail(addr, auth, s.From, []string{to}, []byte(msg))
}

type MockEmailService struct {
	LastTo      string
	LastSubject string
	LastBody    string
}

func (m *MockEmailService) SendVerificationCode(to, code string) error {
	m.LastTo = to
	m.LastSubject = "Verification Code"
	m.LastBody = code
	return nil
}

func (m *MockEmailService) SendPasswordResetCode(to, code string) error {
	m.LastTo = to
	m.LastSubject = "Password Reset Code"
	m.LastBody = code
	return nil
}

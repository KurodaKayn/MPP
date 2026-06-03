package email

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
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
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	var client *smtp.Client
	var err error
	if s.Port == 465 {
		conn, dialErr := tls.Dial("tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, s.Host)
	} else {
		conn, dialErr := net.Dial("tcp", addr)
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, s.Host)
	}
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return err
	}
	if s.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}

	if err := client.Auth(smtp.PlainAuth("", s.From, s.Password, s.Host)); err != nil {
		return err
	}
	if err := client.Mail(s.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(writer, msg); err != nil {
		writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
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

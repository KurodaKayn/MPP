package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
)

//go:embed templates/*.html templates/*.png
var emailTemplateFS embed.FS

const logoContentID = "mpp-logo"

type EmailService interface {
	SendVerificationCode(ctx context.Context, to, code string) error
	SendPasswordResetCode(ctx context.Context, to, code string) error
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

func (s *SMTPEmailService) SendVerificationCode(ctx context.Context, to, code string) error {
	subject := "MPP Registration Verification Code"
	body, err := renderVerificationCodeEmail(verificationCodeEmailData{
		Title:       "Registering Account",
		Description: "Please enter the following verification code on the page to complete verification",
		Code:        code,
		Purpose:     "account registration",
	})
	if err != nil {
		return err
	}
	return s.send(ctx, to, subject, body)
}

func (s *SMTPEmailService) SendPasswordResetCode(ctx context.Context, to, code string) error {
	subject := "MPP Password Reset Verification Code"
	body, err := renderVerificationCodeEmail(verificationCodeEmailData{
		Title:       "Resetting Password",
		Description: "Please enter the following verification code on the page to complete verification",
		Code:        code,
		Purpose:     "password reset",
	})
	if err != nil {
		return err
	}
	return s.send(ctx, to, subject, body)
}

func (s *SMTPEmailService) send(ctx context.Context, to, subject, body string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	msg, err := buildHTMLMessage(s.From, to, subject, body)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	dialer := net.Dialer{}

	var client *smtp.Client
	if s.Port == 465 {
		rawConn, dialErr := dialer.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			return dialErr
		}
		conn := tls.Client(rawConn, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
		if dialErr := conn.Handshake(); dialErr != nil {
			_ = conn.Close()
			return dialErr
		}
		client, err = smtp.NewClient(conn, s.Host)
	} else {
		conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, s.Host)
	}
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.Hello("localhost"); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
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

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.Auth(smtp.PlainAuth("", s.From, s.Password, s.Host)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
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
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
}

func buildHTMLMessage(from, to, subject, body string) (string, error) {
	logoPNG, err := emailTemplateFS.ReadFile("templates/mpp-with-name-white.png")
	if err != nil {
		return "", err
	}

	var msg bytes.Buffer
	writer := multipart.NewWriter(&msg)

	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/related; boundary=%q; type=%q\r\n", writer.Boundary(), "text/html")
	fmt.Fprintf(&msg, "\r\n")

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	htmlPart, err := writer.CreatePart(htmlHeader)
	if err != nil {
		return "", err
	}
	quotedHTML := quotedprintable.NewWriter(htmlPart)
	if _, err := quotedHTML.Write([]byte(body)); err != nil {
		_ = quotedHTML.Close()
		return "", err
	}
	if err := quotedHTML.Close(); err != nil {
		return "", err
	}

	logoHeader := textproto.MIMEHeader{}
	logoHeader.Set("Content-Type", `image/png; name="mpp-with-name-white.png"`)
	logoHeader.Set("Content-Transfer-Encoding", "base64")
	logoHeader.Set("Content-ID", fmt.Sprintf("<%s>", logoContentID))
	logoHeader.Set("Content-Disposition", `inline; filename="mpp-with-name-white.png"`)
	logoPart, err := writer.CreatePart(logoHeader)
	if err != nil {
		return "", err
	}
	if err := writeBase64Lines(logoPart, logoPNG); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}
	return msg.String(), nil
}

func writeBase64Lines(writer io.Writer, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		if _, err := fmt.Fprintf(writer, "%s\r\n", encoded[:76]); err != nil {
			return err
		}
		encoded = encoded[76:]
	}
	_, err := fmt.Fprintf(writer, "%s\r\n", encoded)
	return err
}

type verificationCodeEmailData struct {
	Title       string
	Description string
	Code        string
	Purpose     string
}

func renderVerificationCodeEmail(data verificationCodeEmailData) (string, error) {
	tmpl, err := template.ParseFS(emailTemplateFS, "templates/verification_code.html")
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return "", err
	}
	return body.String(), nil
}

type MockEmailService struct {
	LastTo      string
	LastSubject string
	LastBody    string
}

func (m *MockEmailService) SendVerificationCode(_ context.Context, to, code string) error {
	m.LastTo = to
	m.LastSubject = "MPP Registration Verification Code"
	m.LastBody = strings.TrimSpace(code)
	return nil
}

func (m *MockEmailService) SendPasswordResetCode(_ context.Context, to, code string) error {
	m.LastTo = to
	m.LastSubject = "MPP Password Reset Verification Code"
	m.LastBody = strings.TrimSpace(code)
	return nil
}

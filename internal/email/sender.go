package email

import (
	// "fmt"
	"net/smtp"
	"os"
)

func SendEmail(to, subject, body string) error {
	from := os.Getenv("EMAIL_FROM")
	password := os.Getenv("EMAIL_PASSWORD")
	host := os.Getenv("EMAIL_HOST")
	port := os.Getenv("EMAIL_PORT")

	auth := smtp.PlainAuth("", from, password, host)
	msg := []byte("Subject: " + subject + "\r\n" +
		"From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n" +
		body)

	addr := host + ":" + port
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

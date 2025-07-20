package services

import (
	"fmt"
	"math/rand"
	"mini-blog/app/config"
	"strconv"
	"time"

	"github.com/resend/resend-go/v2"
)

type EmailService struct {
	client *resend.Client
	cfg    *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	client := resend.NewClient(cfg.Auth.ResendAPIKey)
	return &EmailService{
		client: client,
		cfg:    cfg,
	}
}

func (e *EmailService) SendOTP(email, name, otp string) error {
	if e.cfg.Auth.ResendAPIKey == "" {
		return nil
	}

	params := &resend.SendEmailRequest{
		From:    "NODELIKE <onboarding@nodelike.com>",
		To:      []string{email},
		Subject: "Your NODELIKE Verification Code",
		Html: fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
			<h2 style="color: #333;">Welcome to NODELIKE!</h2>
			<p>Hi %s,</p>
			<p>Thank you for signing up! Please use the following verification code to complete your registration:</p>
			<div style="background-color: #f8f9fa; padding: 20px; border-radius: 8px; text-align: center; margin: 20px 0;">
				<h1 style="color: #007bff; letter-spacing: 4px; margin: 0;">%s</h1>
			</div>
			<p>This code will expire in 10 minutes.</p>
			<p>If you didn't request this code, please ignore this email.</p>
			<p>Best regards,<br>NODELIKE Team</p>
		</div>
		`, name, otp),
	}

	_, err := e.client.Emails.Send(params)
	return err
}

func GenerateOTP() string {
	rand.Seed(time.Now().UnixNano())
	otp := rand.Intn(900000) + 100000
	return strconv.Itoa(otp)
}

func (e *EmailService) SendWelcomeEmail(email, name string, isAdmin bool) error {
	if e.cfg.Auth.ResendAPIKey == "" {
		fmt.Printf("✅ Welcome email for %s (Admin: %t)\n", email, isAdmin)
		return nil
	}

	adminMessage := ""
	if isAdmin {
		adminMessage = "<p><strong>🎉 You have been granted admin privileges!</strong></p>"
	}

	params := &resend.SendEmailRequest{
		From:    "NODELIKE <onboarding@nodelike.com>",
		To:      []string{email},
		Subject: "Welcome to NODELIKE!",
		Html: fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
			<h2 style="color: #333;">Welcome to NODELIKE, %s!</h2>
			<p>Your account has been successfully verified and activated.</p>
			%s
			<p>You can now start exploring and creating amazing content!</p>
			<div style="text-align: center; margin: 30px 0;">
				<a href="http://localhost:8080" style="background-color: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">
					Visit NODELIKE
				</a>
			</div>
			<p>Best regards,<br>NODELIKE Team</p>
		</div>
		`, name, adminMessage),
	}

	_, err := e.client.Emails.Send(params)
	return err
}

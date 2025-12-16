package services

import (
	"fmt"
	"log"

	"github.com/resend/resend-go/v2"
	"github.com/tomr1233/intake-form-api/internal/config"
	"github.com/tomr1233/intake-form-api/internal/models"
)

// EmailService handles sending email notifications.
type EmailService struct {
	client  *resend.Client
	config  config.EmailConfig
	enabled bool
}

// NewEmailService creates a new EmailService.
// If API key or notification email is empty, the service will be disabled.
func NewEmailService(cfg config.EmailConfig) *EmailService {
	enabled := cfg.ResendAPIKey != "" && cfg.NotificationEmail != ""

	var client *resend.Client
	if enabled {
		client = resend.NewClient(cfg.ResendAPIKey)
	}

	return &EmailService{
		client:  client,
		config:  cfg,
		enabled: enabled,
	}
}

// IsEnabled returns whether the email service is configured and enabled.
func (e *EmailService) IsEnabled() bool {
	return e.enabled
}

// SendSubmissionNotificationAsync sends an email notification asynchronously.
// Errors are logged but do not propagate - this is fire-and-forget.
func (e *EmailService) SendSubmissionNotificationAsync(submission *models.Submission) {
	if !e.enabled {
		return
	}

	go func() {
		if err := e.sendSubmissionNotification(submission); err != nil {
			log.Printf("Failed to send submission notification email: %v", err)
		}
	}()
}

// sendSubmissionNotification sends the email (internal, synchronous).
func (e *EmailService) sendSubmissionNotification(submission *models.Submission) error {
	subject := fmt.Sprintf("New Form Submission: %s %s", submission.FirstName, submission.LastName)
	if submission.CompanyName != "" {
		subject = fmt.Sprintf("%s - %s", subject, submission.CompanyName)
	}

	adminURL := fmt.Sprintf("%s/admin/%s", e.config.BaseURL, submission.AdminToken)

	body := e.buildEmailBody(submission, adminURL)

	params := &resend.SendEmailRequest{
		From:    "Form Submissions <onboarding@resend.dev>",
		To:      []string{e.config.NotificationEmail},
		Subject: subject,
		Text:    body,
	}

	_, err := e.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("resend API error: %w", err)
	}

	log.Printf("Submission notification email sent for %s", submission.ID)
	return nil
}

// buildEmailBody creates the plain text email content.
func (e *EmailService) buildEmailBody(submission *models.Submission, adminURL string) string {
	body := fmt.Sprintf(`New intake form submission received!

CONTACT
-------
Name: %s %s
Email: %s`, submission.FirstName, submission.LastName, submission.Email)

	if submission.CompanyName != "" {
		body += fmt.Sprintf("\nCompany: %s", submission.CompanyName)
	}
	if submission.Website != "" {
		body += fmt.Sprintf("\nWebsite: %s", submission.Website)
	}

	body += "\n\nCONTEXT\n-------"
	if submission.ReasonForBooking != "" {
		body += fmt.Sprintf("\nReason for Booking:\n%s", submission.ReasonForBooking)
	}
	if submission.HowDidYouHear != "" {
		body += fmt.Sprintf("\n\nHow They Heard About You: %s", submission.HowDidYouHear)
	}

	body += fmt.Sprintf(`

QUICK STATS
-----------
Current Revenue: %s
Team Size: %s
Marketing Budget: %s
Decision Maker: %s

VIEW FULL DETAILS & AI ANALYSIS
-------------------------------
%s

---
Submission ID: %s
Submitted at: %s
`,
		valueOrNA(submission.CurrentRevenue),
		valueOrNA(submission.TeamSize),
		valueOrNA(submission.MarketingBudget),
		valueOrNA(submission.IsDecisionMaker),
		adminURL,
		submission.ID,
		submission.CreatedAt.Format("Jan 2, 2006 at 3:04 PM"),
	)

	return body
}

func valueOrNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

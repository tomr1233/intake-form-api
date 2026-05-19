package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventTypeFormSubmission is the only supported event type for now.
const EventTypeFormSubmission = "form.submission"

// KnownEventTypes is the allow-list used by request validation.
var KnownEventTypes = map[string]bool{
	EventTypeFormSubmission: true,
}

// DeliveryStatus enumerates webhook delivery lifecycle states.
type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySuccess DeliveryStatus = "success"
	DeliveryFailed  DeliveryStatus = "failed"
)

// Webhook is an outbound subscription.
type Webhook struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Secret       string    `json:"-"` // never serialized
	SecretPrefix string    `json:"secretPrefix"`
	Events       []string  `json:"events"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// WebhookDelivery records one delivery attempt sequence for a single event.
type WebhookDelivery struct {
	ID               uuid.UUID       `json:"id"`
	WebhookID        uuid.UUID       `json:"webhookId"`
	EventType        string          `json:"eventType"`
	EventID          uuid.UUID       `json:"eventId"`
	Payload          json.RawMessage `json:"payload"`
	Status           DeliveryStatus  `json:"status"`
	Attempts         int             `json:"attempts"`
	LastStatusCode   *int            `json:"lastStatusCode,omitempty"`
	LastError        *string         `json:"lastError,omitempty"`
	LastResponseBody *string         `json:"lastResponseBody,omitempty"`
	NextAttemptAt    *time.Time      `json:"nextAttemptAt,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// CreateWebhookRequest is the JSON body for POST /api/webhooks.
type CreateWebhookRequest struct {
	Name   string   `json:"name" binding:"required,min=1,max=100"`
	URL    string   `json:"url" binding:"required,max=2048"`
	Events []string `json:"events"`
	Active *bool    `json:"active,omitempty"` // pointer so omission => default true
}

// UpdateWebhookRequest is the JSON body for PATCH /api/webhooks/:id.
// All fields are optional; nil means "leave unchanged".
type UpdateWebhookRequest struct {
	Name   *string  `json:"name,omitempty" binding:"omitempty,min=1,max=100"`
	URL    *string  `json:"url,omitempty" binding:"omitempty,max=2048"`
	Events []string `json:"events,omitempty"`
	Active *bool    `json:"active,omitempty"`
}

// WebhookResponse is the standard read-only representation of a webhook.
// It deliberately has no Secret field so handlers cannot accidentally leak it.
type WebhookResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	SecretPrefix string    `json:"secretPrefix"`
	Events       []string  `json:"events"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ToResponse converts a Webhook to its safe, secret-free response shape.
func (w *Webhook) ToResponse() WebhookResponse {
	return WebhookResponse{
		ID:           w.ID,
		Name:         w.Name,
		URL:          w.URL,
		SecretPrefix: w.SecretPrefix,
		Events:       w.Events,
		Active:       w.Active,
		CreatedAt:    w.CreatedAt,
		UpdatedAt:    w.UpdatedAt,
	}
}

// WebhookWithSecretResponse is returned ONLY by create and rotate-secret.
type WebhookWithSecretResponse struct {
	WebhookResponse
	Secret string `json:"secret"`
}

// DeliverySummary is the list-endpoint shape (no payload, no body).
type DeliverySummary struct {
	ID             uuid.UUID      `json:"id"`
	WebhookID      uuid.UUID      `json:"webhookId"`
	EventType      string         `json:"eventType"`
	EventID        uuid.UUID      `json:"eventId"`
	Status         DeliveryStatus `json:"status"`
	Attempts       int            `json:"attempts"`
	LastStatusCode *int           `json:"lastStatusCode,omitempty"`
	NextAttemptAt  *time.Time     `json:"nextAttemptAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// FormSubmissionPayload is the envelope sent to receivers.
type FormSubmissionPayload struct {
	ID        uuid.UUID   `json:"id"` // delivery id
	Event     string      `json:"event"`
	CreatedAt time.Time   `json:"createdAt"`
	Data      *Submission `json:"data"`
}

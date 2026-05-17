package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tomr1233/intake-form-api/internal/models"
	"github.com/tomr1233/intake-form-api/internal/repository"
)

// CreateWebhook handles POST /api/webhooks.
func (h *Handler) CreateWebhook(c *gin.Context) {
	var req models.CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if err := validateWebhookURLString(req.URL); err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateEvents(req.Events); err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, err.Error())
		return
	}

	events := req.Events
	if len(events) == 0 {
		events = []string{models.EventTypeFormSubmission}
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	secret, err := generateWebhookSecret()
	if err != nil {
		h.internalError(c)
		return
	}
	now := time.Now()
	w := &models.Webhook{
		ID:           uuid.New(),
		Name:         req.Name,
		URL:          req.URL,
		Secret:       secret,
		SecretPrefix: secret[:8],
		Events:       events,
		Active:       active,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := h.webhooks.Create(c.Request.Context(), w); err != nil {
		h.internalError(c)
		return
	}
	h.respondData(c, http.StatusCreated, models.WebhookWithSecretResponse{
		WebhookResponse: w.ToResponse(),
		Secret:          secret,
	})
}

// ListWebhooks handles GET /api/webhooks.
func (h *Handler) ListWebhooks(c *gin.Context) {
	hooks, err := h.webhooks.List(c.Request.Context())
	if err != nil {
		h.internalError(c)
		return
	}
	responses := make([]models.WebhookResponse, len(hooks))
	for i, w := range hooks {
		responses[i] = w.ToResponse()
	}
	h.respondData(c, http.StatusOK, responses)
}

// GetWebhook handles GET /api/webhooks/:id.
func (h *Handler) GetWebhook(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "invalid id")
		return
	}
	w, err := h.webhooks.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.notFound(c)
			return
		}
		h.internalError(c)
		return
	}
	h.respondData(c, http.StatusOK, w.ToResponse())
}

// UpdateWebhook handles PATCH /api/webhooks/:id.
func (h *Handler) UpdateWebhook(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req models.UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if req.URL != nil {
		if err := validateWebhookURLString(*req.URL); err != nil {
			h.respondErrorSimple(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := validateEvents(req.Events); err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, err.Error())
		return
	}

	upd := repository.WebhookUpdate{
		Name:   req.Name,
		URL:    req.URL,
		Events: req.Events,
		Active: req.Active,
	}
	w, err := h.webhooks.Update(c.Request.Context(), id, upd)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.notFound(c)
			return
		}
		h.internalError(c)
		return
	}
	h.respondData(c, http.StatusOK, w.ToResponse())
}

// RotateWebhookSecret handles POST /api/webhooks/:id/rotate-secret.
func (h *Handler) RotateWebhookSecret(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "invalid id")
		return
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		h.internalError(c)
		return
	}
	w, err := h.webhooks.RotateSecret(c.Request.Context(), id, secret, secret[:8])
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.notFound(c)
			return
		}
		h.internalError(c)
		return
	}
	h.respondData(c, http.StatusOK, models.WebhookWithSecretResponse{
		WebhookResponse: w.ToResponse(),
		Secret:          secret,
	})
}

// DeleteWebhook handles DELETE /api/webhooks/:id.
func (h *Handler) DeleteWebhook(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.webhooks.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.notFound(c)
			return
		}
		h.internalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// generateWebhookSecret returns 32 random bytes hex-encoded (64 chars).
func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// validateWebhookURLString enforces http/https scheme, non-empty host, and
// no userinfo. Matches the dispatcher's URL contract.
func validateWebhookURLString(rawURL string) error {
	if rawURL == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("url scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("url host is required")
	}
	if u.User != nil {
		return errors.New("url must not contain userinfo")
	}
	return nil
}

// validateEvents enforces non-empty + known event types. Pass nil to skip.
func validateEvents(events []string) error {
	if events == nil {
		return nil
	}
	if len(events) == 0 {
		return errors.New("events must not be empty")
	}
	for _, e := range events {
		if !models.KnownEventTypes[e] {
			return errors.New("unknown event type: " + e)
		}
	}
	return nil
}

// parseListLimit parses the optional ?limit= query param, clamped to [1,200].
// Returns 50 if missing or invalid.
func parseListLimit(c *gin.Context) int {
	raw := c.Query("limit")
	if raw == "" {
		return 50
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

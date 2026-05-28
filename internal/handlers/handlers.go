package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tomr1233/intake-form-api/internal/config"
	"github.com/tomr1233/intake-form-api/internal/repository"
	"github.com/tomr1233/intake-form-api/internal/services"
)

type Handler struct {
	submissions        repository.SubmissionRepository
	analysis           repository.AnalysisRepository
	analyzer           *services.Analyzer
	email              *services.EmailService
	config             *config.Config
	webhooks           repository.WebhookRepository
	webhookDeliveries  repository.WebhookDeliveryRepository
	webhookDispatcher  *services.Dispatcher
}

func NewHandler(
	submissions repository.SubmissionRepository,
	analysis repository.AnalysisRepository,
	analyzer *services.Analyzer,
	email *services.EmailService,
	cfg *config.Config,
	webhooks repository.WebhookRepository,
	webhookDeliveries repository.WebhookDeliveryRepository,
	webhookDispatcher *services.Dispatcher,
) *Handler {
	return &Handler{
		submissions:       submissions,
		analysis:          analysis,
		analyzer:          analyzer,
		email:             email,
		config:            cfg,
		webhooks:          webhooks,
		webhookDeliveries: webhookDeliveries,
		webhookDispatcher: webhookDispatcher,
	}
}

// Response is the standard JSON response format.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (h *Handler) respondJSON(c *gin.Context, status int, data interface{}) {
	c.JSON(status, Response{Success: true, Data: data})
}

func (h *Handler) respondError(c *gin.Context, status int, message string) {
	c.JSON(status, Response{Success: false, Error: message})
}

func (h *Handler) respondData(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

func (h *Handler) respondErrorSimple(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func (h *Handler) notFound(c *gin.Context) {
	h.respondErrorSimple(c, http.StatusNotFound, "not found")
}

func (h *Handler) internalError(c *gin.Context) {
	h.respondErrorSimple(c, http.StatusInternalServerError, "internal server error")
}

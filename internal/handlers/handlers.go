package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tomr1233/intake-form-api/internal/config"
	"github.com/tomr1233/intake-form-api/internal/repository"
	"github.com/tomr1233/intake-form-api/internal/services"
)

// Handler holds all dependencies for HTTP handlers.
type Handler struct {
	submissions repository.SubmissionRepository
	analysis    repository.AnalysisRepository
	analyzer    *services.Analyzer
	config      *config.Config
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(
	submissions repository.SubmissionRepository,
	analysis repository.AnalysisRepository,
	analyzer *services.Analyzer,
	cfg *config.Config,
) *Handler {
	return &Handler{
		submissions: submissions,
		analysis:    analysis,
		analyzer:    analyzer,
		config:      cfg,
	}
}

// Response is the standard JSON response format.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// respondJSON sends a successful JSON response.
func (h *Handler) respondJSON(c *gin.Context, status int, data interface{}) {
	c.JSON(status, Response{Success: true, Data: data})
}

// respondError sends an error JSON response.
func (h *Handler) respondError(c *gin.Context, status int, message string) {
	c.JSON(status, Response{Success: false, Error: message})
}

// respondData sends data directly without wrapping (for API compatibility).
func (h *Handler) respondData(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// respondErrorSimple sends a simple error response.
func (h *Handler) respondErrorSimple(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// notFound sends a 404 response.
func (h *Handler) notFound(c *gin.Context) {
	h.respondErrorSimple(c, http.StatusNotFound, "not found")
}

// internalError sends a 500 response.
func (h *Handler) internalError(c *gin.Context) {
	h.respondErrorSimple(c, http.StatusInternalServerError, "internal server error")
}

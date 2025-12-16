package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tomr1233/intake-form-api/internal/models"
	"github.com/tomr1233/intake-form-api/pkg/tokens"
)

// CreateSubmissionResponse is the response for creating a submission.
type CreateSubmissionResponse struct {
	ID       uuid.UUID `json:"id"`
	AdminURL string    `json:"adminUrl"`
}

// CreateSubmission handles POST /api/submissions.
func (h *Handler) CreateSubmission(c *gin.Context) {
	var req models.CreateSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErrorSimple(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	// Generate secure admin token
	adminToken, err := tokens.GenerateSecureToken(32)
	if err != nil {
		h.internalError(c)
		return
	}

	// Create submission model
	submissionID := uuid.New()
	submission := req.ToSubmission(submissionID, adminToken)

	// Save to database
	if err := h.submissions.Create(c.Request.Context(), submission); err != nil {
		h.internalError(c)
		return
	}

	// Send email notification (async, fire and forget)
	h.email.SendSubmissionNotificationAsync(submission)

	// Trigger async analysis (fire and forget)
	h.analyzer.AnalyzeAsync(submission)

	// Return immediately with submission ID and admin URL
	h.respondData(c, http.StatusCreated, CreateSubmissionResponse{
		ID:       submissionID,
		AdminURL: fmt.Sprintf("/admin/%s", adminToken),
	})
}

package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tomr1233/intake-form-api/internal/models"
	"github.com/tomr1233/intake-form-api/internal/repository"
)

// GetAdminResults handles GET /api/admin/:token.
func (h *Handler) GetAdminResults(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		h.respondErrorSimple(c, http.StatusBadRequest, "token is required")
		return
	}

	// Get submission by admin token
	submission, err := h.submissions.GetByAdminToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.notFound(c)
			return
		}
		h.internalError(c)
		return
	}

	// Get analysis result (may not exist yet)
	var analysis *models.AnalysisResult
	analysisResult, err := h.analysis.GetBySubmissionID(c.Request.Context(), submission.ID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		h.internalError(c)
		return
	}
	if err == nil {
		analysis = analysisResult
	}

	// Return response
	h.respondData(c, http.StatusOK, models.AdminResponse{
		Submission: submission,
		Analysis:   analysis,
		Status:     submission.Status,
		CreatedAt:  submission.CreatedAt,
	})
}

// GetAdminStatus handles GET /api/admin/:token/status (lightweight polling).
func (h *Handler) GetAdminStatus(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		h.respondErrorSimple(c, http.StatusBadRequest, "token is required")
		return
	}

	// Get submission by admin token
	submission, err := h.submissions.GetByAdminToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.notFound(c)
			return
		}
		h.internalError(c)
		return
	}

	response := models.AdminStatusResponse{
		Status: submission.Status,
	}

	// If completed, include fit score
	if submission.Status == models.StatusCompleted {
		analysis, err := h.analysis.GetBySubmissionID(c.Request.Context(), submission.ID)
		if err == nil && analysis.EstimatedFitScore > 0 {
			response.EstimatedFitScore = &analysis.EstimatedFitScore
		}
	}

	h.respondData(c, http.StatusOK, response)
}

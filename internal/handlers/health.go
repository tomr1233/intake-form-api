package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tomr1233/intake-form-api/internal/database"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	db *database.DB
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *database.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health returns the health status of the service.
func (h *HealthHandler) Health(c *gin.Context) {
	// Check database connection
	if err := h.db.Health(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "unhealthy",
			"database": "disconnected",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "healthy",
		"database": "connected",
	})
}

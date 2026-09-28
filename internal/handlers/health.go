package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health reports that the process is up. It deliberately does not query the
// database: container probes run every 30s and would keep the Neon compute
// from ever scaling to zero. database.New pings once at startup instead.
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

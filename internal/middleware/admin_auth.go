package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminAuth returns a Gin middleware that enforces a static bearer token.
//
// If expectedKey is empty the middleware short-circuits every request with
// 503 — the feature is disabled. This is intentional: an empty key must not
// silently allow access.
func AdminAuth(expectedKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(expectedKey) == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "admin api key not configured",
			})
			return
		}

		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		got := strings.TrimPrefix(header, prefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(expectedKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

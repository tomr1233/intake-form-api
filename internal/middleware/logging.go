package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logging returns a middleware that logs HTTP requests.
func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Printf("%s %s %d %v", method, path, status, latency)
	}
}

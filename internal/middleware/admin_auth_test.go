package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		configured string
		header     string
		wantStatus int
	}{
		{"no header", "secret", "", http.StatusUnauthorized},
		{"wrong key", "secret", "Bearer nope", http.StatusUnauthorized},
		{"correct key", "secret", "Bearer secret", http.StatusOK},
		{"missing bearer prefix", "secret", "secret", http.StatusUnauthorized},
		{"empty config disables feature", "", "Bearer anything", http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(AdminAuth(tc.configured))
			r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tomr1233/intake-form-api/internal/config"
	"github.com/tomr1233/intake-form-api/internal/database"
	"github.com/tomr1233/intake-form-api/internal/handlers"
	"github.com/tomr1233/intake-form-api/internal/middleware"
	"github.com/tomr1233/intake-form-api/internal/observability"
	"github.com/tomr1233/intake-form-api/internal/repository"
	"github.com/tomr1233/intake-form-api/internal/services"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

const serviceName = "intake-form-api"

// serviceVersion is overridden at build time via -ldflags "-X main.serviceVersion=...".
var serviceVersion = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	gin.SetMode(cfg.Server.GinMode)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Langfuse/OTEL tracing. No-op when LANGFUSE_PUBLIC_KEY /
	// LANGFUSE_SECRET_KEY are unset.
	shutdownTracer, tracingEnabled, err := observability.Setup(ctx, serviceName, serviceVersion)
	if err != nil {
		log.Fatalf("Failed to initialize tracing: %v", err)
	}
	if tracingEnabled {
		log.Println("Langfuse tracing enabled")
	} else {
		log.Println("Langfuse tracing disabled (LANGFUSE_PUBLIC_KEY/LANGFUSE_SECRET_KEY not set)")
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		if err := shutdownTracer(flushCtx); err != nil {
			log.Printf("Error flushing traces: %v", err)
		}
	}()

	// Initialize database
	log.Println("Connecting to database...")
	db, err := database.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Database connected successfully")

	log.Println("Initializing Gemini client...")
	geminiClient, err := services.NewGeminiClient(ctx, cfg.Gemini.APIKey)
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}
	defer func() {
		if err := geminiClient.Close(); err != nil {
			log.Printf("Error closing Gemini client: %v", err)
		}
	}()
	log.Println("Gemini client initialized successfully")

	// Repositories
	submissionRepo := repository.NewSubmissionRepository(db)
	analysisRepo := repository.NewAnalysisRepository(db)
	webhookRepo := repository.NewWebhookRepository(db)
	webhookDeliveryRepo := repository.NewWebhookDeliveryRepository(db)

	// Services
	analyzer := services.NewAnalyzer(geminiClient, submissionRepo, analysisRepo)
	emailService := services.NewEmailService(cfg.Email)
	if emailService.IsEnabled() {
		log.Println("Email notifications enabled")
	} else {
		log.Println("Email notifications disabled (RESEND_API_KEY or NOTIFICATION_EMAIL not set)")
	}
	dispatcher := services.NewDispatcher(webhookRepo, webhookDeliveryRepo, services.DispatcherOptions{
		AllowPrivateIPs: cfg.Webhook.AllowPrivateIPs,
	})
	if cfg.Webhook.AdminAPIKey == "" {
		log.Println("Webhook admin API disabled (ADMIN_API_KEY not set) — /api/webhooks routes will return 503")
	} else {
		log.Println("Webhook admin API enabled")
	}

	// Handlers
	handler := handlers.NewHandler(
		submissionRepo, analysisRepo, analyzer, emailService, cfg,
		webhookRepo, webhookDeliveryRepo, dispatcher,
	)
	healthHandler := handlers.NewHealthHandler(db)

	// Router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(otelgin.Middleware(serviceName))
	router.Use(middleware.Logging())
	router.Use(middleware.CORS(cfg.Server.FrontendURL))

	router.GET("/health", healthHandler.Health)

	api := router.Group("/api")
	{
		api.POST("/submissions", handler.CreateSubmission)
		api.GET("/admin/:token", handler.GetAdminResults)
		api.GET("/admin/:token/status", handler.GetAdminStatus)

		// Webhook management — admin-key protected.
		webhooks := api.Group("/webhooks")
		webhooks.Use(middleware.AdminAuth(cfg.Webhook.AdminAPIKey))
		{
			webhooks.POST("", handler.CreateWebhook)
			webhooks.GET("", handler.ListWebhooks)
			webhooks.GET("/:id", handler.GetWebhook)
			webhooks.PATCH("/:id", handler.UpdateWebhook)
			webhooks.DELETE("/:id", handler.DeleteWebhook)
			webhooks.POST("/:id/rotate-secret", handler.RotateWebhookSecret)
			webhooks.GET("/:id/deliveries", handler.ListWebhookDeliveries)
			webhooks.GET("/:id/deliveries/:deliveryId", handler.GetWebhookDelivery)
		}
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.Server.Port)
		log.Printf("Frontend URL: %s", cfg.Server.FrontendURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	log.Println("Server exited")
}

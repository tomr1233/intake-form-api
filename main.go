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
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set Gin mode
	gin.SetMode(cfg.Server.GinMode)

	// Set up context with cancellation for graceful shutdown
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

	// Initialize Gemini client
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

	// Initialize repositories
	submissionRepo := repository.NewSubmissionRepository(db)
	analysisRepo := repository.NewAnalysisRepository(db)

	// Initialize analyzer service
	analyzer := services.NewAnalyzer(geminiClient, submissionRepo, analysisRepo)

	// Initialize email service
	emailService := services.NewEmailService(cfg.Email)
	if emailService.IsEnabled() {
		log.Println("Email notifications enabled")
	} else {
		log.Println("Email notifications disabled (RESEND_API_KEY or NOTIFICATION_EMAIL not set)")
	}

	// Initialize handlers
	handler := handlers.NewHandler(submissionRepo, analysisRepo, analyzer, emailService, cfg)
	healthHandler := handlers.NewHealthHandler(db)

	// Set up Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(otelgin.Middleware(serviceName))
	router.Use(middleware.Logging())
	router.Use(middleware.CORS(cfg.Server.FrontendURL))

	// Routes
	router.GET("/health", healthHandler.Health)

	api := router.Group("/api")
	{
		api.POST("/submissions", handler.CreateSubmission)
		api.GET("/admin/:token", handler.GetAdminResults)
		api.GET("/admin/:token/status", handler.GetAdminStatus)
	}

	// Start server with graceful shutdown
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on port %s", cfg.Server.Port)
		log.Printf("Frontend URL: %s", cfg.Server.FrontendURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests 5 seconds to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

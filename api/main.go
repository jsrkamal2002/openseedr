package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openseedr/api/db"
	"github.com/openseedr/api/handlers"
	"github.com/openseedr/api/middleware"
	"github.com/openseedr/api/observability"
	"github.com/openseedr/api/services"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Load .env in development
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── OpenTelemetry ─────────────────────────────────────────────────────────
	providers, err := observability.Init(ctx)
	if err != nil {
		slog.Error("failed to initialise OTel", "error", err)
		os.Exit(1)
	}
	defer providers.Shutdown(context.Background())

	if err := observability.InitMetrics(); err != nil {
		slog.Error("failed to initialise metrics", "error", err)
		os.Exit(1)
	}

	// ── Database ──────────────────────────────────────────────────────────────
	db.Connect()

	// ── qBittorrent client ────────────────────────────────────────────────────
	services.QBClient = services.NewQBittorrentClient()

	// ── Gin router ────────────────────────────────────────────────────────────
	if os.Getenv("APP_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Global middleware (order matters)
	r.Use(observability.RecoveryMiddleware())  // panic → 500 + span error
	r.Use(observability.OtelMiddleware())      // creates span per request
	r.Use(observability.MetricsMiddleware())   // records HTTP metrics + access log
	r.Use(corsMiddleware())

	// ── Health & metrics endpoints ────────────────────────────────────────────
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler())) // Prometheus scrape endpoint

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// Auth (public)
	auth := v1.Group("/auth")
	{
		auth.POST("/register", handlers.Register)
		auth.POST("/login", handlers.Login)
		auth.GET("/google", handlers.OAuthRedirectGoogle)
		auth.GET("/google/callback", handlers.OAuthCallbackGoogle)
		auth.GET("/github", handlers.OAuthRedirectGitHub)
		auth.GET("/github/callback", handlers.OAuthCallbackGitHub)
	}

	// Protected routes
	protected := v1.Group("/")
	protected.Use(middleware.Auth())
	{
		// Current user
		protected.GET("/auth/me", handlers.Me)

		// Torrents
		torrents := protected.Group("/torrents")
		{
			torrents.GET("", handlers.ListTorrents)
			torrents.POST("/magnet", handlers.AddMagnet)
			torrents.POST("/file", handlers.AddTorrentFile)
			torrents.GET("/:id", handlers.GetTorrent)
			torrents.GET("/:id/files", handlers.GetTorrentFiles)
			torrents.DELETE("/:id", handlers.DeleteTorrent)
			torrents.POST("/:id/pause", handlers.PauseTorrent)
			torrents.POST("/:id/resume", handlers.ResumeTorrent)
		}

		// Files
		files := protected.Group("/files")
		{
			files.GET("", handlers.ListFiles)
			files.GET("/download", handlers.DownloadFile)
			files.DELETE("", handlers.DeleteFileHandler)
			files.GET("/storage", handlers.StorageInfo)
		}
	}

	// ── HTTP server with graceful shutdown ────────────────────────────────────
	port := observability.GetEnvOrDefault("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "error", err)
	}
	slog.Info("server stopped")
}

func corsMiddleware() gin.HandlerFunc {
	allowedOrigin := observability.GetEnvOrDefault("CORS_ORIGIN", "http://localhost:5173")
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

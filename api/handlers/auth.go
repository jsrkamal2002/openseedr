package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openseedr/api/db"
	"github.com/openseedr/api/middleware"
	"github.com/openseedr/api/models"
	"github.com/openseedr/api/observability"
	"github.com/openseedr/api/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var authTracer = otel.Tracer("openseedr/handlers/auth")

// Register handles POST /api/v1/auth/register
func Register(c *gin.Context) {
	ctx, span := authTracer.Start(c.Request.Context(), "auth.register")
	defer span.End()

	var req struct {
		Email    string `json:"email"    binding:"required,email"`
		Username string `json:"username" binding:"required,min=3,max=32"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		span.SetStatus(codes.Error, "bad request")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	span.SetAttributes(attribute.String("user.email", req.Email))

	// Check duplicate email / username
	var existing models.User
	if err := db.DB.Where("email = ? OR username = ?", req.Email, req.Username).First(&existing).Error; err == nil {
		span.SetStatus(codes.Error, "duplicate user")
		c.JSON(http.StatusConflict, gin.H{"error": "email or username already taken", "trace_id": observability.TraceID(ctx)})
		return
	}

	hash, err := services.HashPassword(req.Password)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process password", "trace_id": observability.TraceID(ctx)})
		return
	}

	user := &models.User{
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: hash,
		Provider:     "local",
	}
	if err := db.DB.Create(user).Error; err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "db error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user", "trace_id": observability.TraceID(ctx)})
		return
	}

	if err := services.EnsureUserDir(user.ID.String()); err != nil {
		slog.WarnContext(ctx, "failed to create user storage dir", "user_id", user.ID, "error", err)
	}

	token, err := services.GenerateJWT(user.ID, user.Email, user.Username, user.IsAdmin)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token", "trace_id": observability.TraceID(ctx)})
		return
	}

	observability.RecordLoginAttempt(ctx, "local", true)
	slog.InfoContext(ctx, "user registered", "user_id", user.ID, "email", user.Email)

	c.JSON(http.StatusCreated, gin.H{
		"token": token,
		"user":  user,
	})
}

// Login handles POST /api/v1/auth/login
func Login(c *gin.Context) {
	ctx, span := authTracer.Start(c.Request.Context(), "auth.login")
	defer span.End()

	var req struct {
		Email    string `json:"email"    binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	span.SetAttributes(attribute.String("user.email", req.Email))
	observability.RecordLoginAttempt(ctx, "local", false) // will update below

	var user models.User
	if err := db.DB.Where("email = ? AND provider = 'local'", req.Email).First(&user).Error; err != nil {
		observability.RecordLoginAttempt(ctx, "local", false)
		span.SetStatus(codes.Error, "user not found")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials", "trace_id": observability.TraceID(ctx)})
		return
	}

	if !services.CheckPassword(req.Password, user.PasswordHash) {
		observability.RecordLoginAttempt(ctx, "local", false)
		span.SetStatus(codes.Error, "wrong password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials", "trace_id": observability.TraceID(ctx)})
		return
	}

	token, err := services.GenerateJWT(user.ID, user.Email, user.Username, user.IsAdmin)
	if err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token", "trace_id": observability.TraceID(ctx)})
		return
	}

	observability.RecordLoginAttempt(ctx, "local", true)
	slog.InfoContext(ctx, "user logged in", "user_id", user.ID)

	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// Me handles GET /api/v1/auth/me
func Me(c *gin.Context) {
	ctx, span := authTracer.Start(c.Request.Context(), "auth.me")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found", "trace_id": observability.TraceID(ctx)})
		return
	}

	// StorageUsed in the DB is never updated; compute the real value from disk
	// so the client always gets an accurate balance.
	if used, err := services.DirSize(services.UserStoragePath(userID)); err == nil {
		user.StorageUsed = used
	}

	c.JSON(http.StatusOK, user)
}

// OAuthRedirectGoogle handles GET /api/v1/auth/google
func OAuthRedirectGoogle(c *gin.Context) {
	cfg := services.GoogleOAuthConfig()
	state := uuid.New().String()
	// In production store state in a short-lived cookie/session for CSRF protection
	c.SetCookie("oauth_state", state, 300, "/", "", true, true)
	c.Redirect(http.StatusTemporaryRedirect, cfg.AuthCodeURL(state))
}

// OAuthCallbackGoogle handles GET /api/v1/auth/google/callback
func OAuthCallbackGoogle(c *gin.Context) {
	ctx, span := authTracer.Start(c.Request.Context(), "auth.oauth.google.callback")
	defer span.End()

	oauthUser, token, err := handleOAuthCallback(c, "google")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	observability.RecordLoginAttempt(ctx, "google", true)
	slog.InfoContext(ctx, "oauth login", "provider", "google", "user_id", oauthUser.ID)
	c.JSON(http.StatusOK, gin.H{"token": token, "user": oauthUser})
}

// OAuthRedirectGitHub handles GET /api/v1/auth/github
func OAuthRedirectGitHub(c *gin.Context) {
	cfg := services.GitHubOAuthConfig()
	state := uuid.New().String()
	c.SetCookie("oauth_state", state, 300, "/", "", true, true)
	c.Redirect(http.StatusTemporaryRedirect, cfg.AuthCodeURL(state))
}

// OAuthCallbackGitHub handles GET /api/v1/auth/github/callback
func OAuthCallbackGitHub(c *gin.Context) {
	ctx, span := authTracer.Start(c.Request.Context(), "auth.oauth.github.callback")
	defer span.End()

	oauthUser, token, err := handleOAuthCallback(c, "github")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	observability.RecordLoginAttempt(ctx, "github", true)
	slog.InfoContext(ctx, "oauth login", "provider", "github", "user_id", oauthUser.ID)
	c.JSON(http.StatusOK, gin.H{"token": token, "user": oauthUser})
}

// handleOAuthCallback is shared logic for Google and GitHub OAuth callbacks.
func handleOAuthCallback(c *gin.Context, provider string) (*models.User, string, error) {
	ctx := c.Request.Context()

	// CSRF state check
	stateCookie, _ := c.Cookie("oauth_state")
	if c.Query("state") != stateCookie {
		return nil, "", fmt.Errorf("oauth state mismatch")
	}
	c.SetCookie("oauth_state", "", -1, "/", "", true, true)

	code := c.Query("code")

	var info *services.OAuthUserInfo
	var err error
	switch provider {
	case "google":
		info, err = services.GetGoogleUserInfo(ctx, code)
	case "github":
		info, err = services.GetGitHubUserInfo(ctx, code)
	default:
		return nil, "", fmt.Errorf("unknown provider: %s", provider)
	}
	if err != nil {
		return nil, "", err
	}

	// Upsert user
	var user models.User
	result := db.DB.Where("provider = ? AND provider_id = ?", provider, info.ProviderID).First(&user)
	if result.Error != nil {
		// New user — also check if email already registered locally
		var byEmail models.User
		if db.DB.Where("email = ?", info.Email).First(&byEmail).Error == nil {
			// Merge: attach OAuth to existing account
			byEmail.Provider = provider
			byEmail.ProviderID = info.ProviderID
			byEmail.AvatarURL = info.AvatarURL
			byEmail.UpdatedAt = time.Now()
			db.DB.Save(&byEmail)
			user = byEmail
		} else {
			// Brand new user
			username := sanitizeUsername(info.Username)
			user = models.User{
				Email:      info.Email,
				Username:   username,
				Provider:   provider,
				ProviderID: info.ProviderID,
				AvatarURL:  info.AvatarURL,
			}
			if err := db.DB.Create(&user).Error; err != nil {
				return nil, "", err
			}
			if err := services.EnsureUserDir(user.ID.String()); err != nil {
				slog.WarnContext(ctx, "failed to create user storage dir", "user_id", user.ID)
			}
		}
	}

	token, err := services.GenerateJWT(user.ID, user.Email, user.Username, user.IsAdmin)
	if err != nil {
		return nil, "", err
	}
	return &user, token, nil
}

// ChangePassword handles POST /api/v1/auth/change-password
// Requires the user's current password and a new password (min 8 chars).
// Only works for local (non-OAuth) accounts.
func ChangePassword(c *gin.Context) {
	ctx, span := authTracer.Start(c.Request.Context(), "auth.change_password")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password"     binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found", "trace_id": observability.TraceID(ctx)})
		return
	}

	// OAuth accounts have no password hash — block the operation.
	if user.Provider != "local" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password change is not supported for OAuth accounts"})
		return
	}

	if !services.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		span.SetStatus(codes.Error, "wrong current password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect", "trace_id": observability.TraceID(ctx)})
		return
	}

	newHash, err := services.HashPassword(req.NewPassword)
	if err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process new password", "trace_id": observability.TraceID(ctx)})
		return
	}

	if err := db.DB.Model(&user).Update("password_hash", newHash).Error; err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save new password", "trace_id": observability.TraceID(ctx)})
		return
	}

	slog.InfoContext(ctx, "user changed password", "user_id", userID)
	c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}

func sanitizeUsername(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 32 {
		raw = raw[:32]
	}
	if raw == "" {
		raw = "user_" + uuid.New().String()[:8]
	}
	return raw
}

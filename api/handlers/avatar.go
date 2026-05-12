package handlers

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/openseedr/api/db"
	"github.com/openseedr/api/middleware"
	"github.com/openseedr/api/models"
	"github.com/openseedr/api/observability"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"
)

const maxAvatarSize = 5 << 20 // 5 MB

var allowedAvatarMIME = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

func avatarsDir() string {
	base := os.Getenv("STORAGE_PATH")
	if base == "" {
		base = "/data"
	}
	return filepath.Join(base, ".avatars")
}

// UploadAvatar handles POST /api/v1/auth/avatar
// Accepts multipart/form-data with field "avatar" (image file, max 5 MB).
// Saves the file to /data/.avatars/<userID>.<ext> and updates avatar_url in DB.
func UploadAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(c)

	// Limit body size before parsing multipart
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarSize+1024)

	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "avatar field is required", "trace_id": observability.TraceID(ctx)})
		return
	}
	defer file.Close()

	if header.Size > maxAvatarSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file exceeds 5 MB limit"})
		return
	}

	// Read first 512 bytes to detect MIME type
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read file"})
		return
	}
	mime := http.DetectContentType(buf[:n])
	// Strip params like "; charset=..." if present
	mime = strings.Split(mime, ";")[0]
	ext, ok := allowedAvatarMIME[mime]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported image type: %s", mime)})
		return
	}

	// Seek back and validate it's actually a decodable image
	if _, seekErr := file.(io.Seeker).Seek(0, io.SeekStart); seekErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not process file"})
		return
	}
	if _, _, err := image.DecodeConfig(file); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is not a valid image"})
		return
	}

	// Seek back again to write the full file
	if _, seekErr := file.(io.Seeker).Seek(0, io.SeekStart); seekErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process file"})
		return
	}

	// Ensure avatars directory exists
	dir := avatarsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.ErrorContext(ctx, "failed to create avatars dir", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage error", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Remove any previous avatar for this user (any extension)
	for _, oldExt := range allowedAvatarMIME {
		_ = os.Remove(filepath.Join(dir, userID+"."+oldExt))
	}

	// Write new avatar
	destPath := filepath.Join(dir, userID+"."+ext)
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create avatar file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage error", "trace_id": observability.TraceID(ctx)})
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		slog.ErrorContext(ctx, "failed to write avatar file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage error", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Update DB: avatar_url points to the public serve endpoint
	avatarURL := "/api/v1/avatars/" + userID
	if err := db.DB.Model(&models.User{}).Where("id = ?", userID).Update("avatar_url", avatarURL).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update avatar_url", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Return updated user
	var user models.User
	db.DB.First(&user, "id = ?", userID)
	slog.InfoContext(ctx, "avatar uploaded", "user_id", userID)
	c.JSON(http.StatusOK, user)
}

// ServeAvatar handles GET /api/v1/avatars/:userID (public, no auth required).
// Finds the stored avatar file for the given user and streams it.
func ServeAvatar(c *gin.Context) {
	userID := c.Param("userID")
	// Sanitize: only allow UUID-like characters to prevent traversal
	for _, ch := range userID {
		if !('a' <= ch && ch <= 'z') && !('A' <= ch && ch <= 'Z') &&
			!('0' <= ch && ch <= '9') && ch != '-' {
			c.Status(http.StatusNotFound)
			return
		}
	}

	dir := avatarsDir()
	for mime, ext := range allowedAvatarMIME {
		path := filepath.Join(dir, userID+"."+ext)
		if _, err := os.Stat(path); err == nil {
			c.Header("Cache-Control", "public, max-age=3600")
			c.File(path)
			_ = mime
			return
		}
	}

	c.Status(http.StatusNotFound)
}

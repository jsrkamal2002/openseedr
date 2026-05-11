package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/openseedr/api/middleware"
	"github.com/openseedr/api/observability"
	"github.com/openseedr/api/services"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var fileTracer = otel.Tracer("openseedr/handlers/files")

// ListFiles handles GET /api/v1/files
// Query param: path (optional, defaults to root)
func ListFiles(c *gin.Context) {
	ctx, span := fileTracer.Start(c.Request.Context(), "files.list")
	defer span.End()

	userID := middleware.GetUserID(c)
	subPath := c.DefaultQuery("path", "/")
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("file.path", subPath),
	)

	files, err := services.ListFiles(userID, subPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list error")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to list files",
			"trace_id": observability.TraceID(ctx),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"files": files,
		"path":  subPath,
		"count": len(files),
	})
}

// DownloadFile handles GET /api/v1/files/download
// Query param: path (required)
func DownloadFile(c *gin.Context) {
	ctx, span := fileTracer.Start(c.Request.Context(), "files.download")
	defer span.End()

	userID := middleware.GetUserID(c)
	subPath := c.Query("path")
	if subPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query param required"})
		return
	}

	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("file.path", subPath),
	)

	f, info, err := services.OpenFile(userID, subPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "open error")
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "file not found or access denied",
			"trace_id": observability.TraceID(ctx),
		})
		return
	}
	defer f.Close()

	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download a directory"})
		return
	}

	span.SetAttributes(attribute.Int64("file.size_bytes", info.Size()))
	slog.InfoContext(ctx, "file download",
		"user_id", userID,
		"path", subPath,
		"size", info.Size(),
	)

	// Use RFC 5987 encoded filename to handle unicode/special characters safely.
	safeName := filepath.Base(subPath)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
	// Prevent the browser from sniffing the content type away from octet-stream,
	// which could cause the browser to render HTML/SVG files inline.
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, info.Size(), "application/octet-stream", f, nil)
}

// DeleteFileHandler handles DELETE /api/v1/files
// Query param: path (required)
func DeleteFileHandler(c *gin.Context) {
	ctx, span := fileTracer.Start(c.Request.Context(), "files.delete")
	defer span.End()

	userID := middleware.GetUserID(c)
	subPath := c.Query("path")
	if subPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query param required"})
		return
	}

	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("file.path", subPath),
	)

	if err := services.DeleteFile(userID, subPath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete error")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to delete file",
			"trace_id": observability.TraceID(ctx),
		})
		return
	}

	slog.InfoContext(ctx, "file deleted", "user_id", userID, "path", subPath)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// StorageInfo handles GET /api/v1/files/storage
func StorageInfo(c *gin.Context) {
	ctx, span := fileTracer.Start(c.Request.Context(), "files.storage_info")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	used, err := services.DirSize(services.UserStoragePath(userID))
	if err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to calculate storage",
			"trace_id": observability.TraceID(ctx),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"used_bytes":  used,
		"used_gb":     float64(used) / 1e9,
	})
}


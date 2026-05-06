package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

var torrentTracer = otel.Tracer("openseedr/handlers/torrents")

// ListTorrents handles GET /api/v1/torrents
func ListTorrents(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.list")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	var torrents []models.Torrent
	if err := db.DB.Where("user_id = ?", userID).Order("created_at desc").Find(&torrents).Error; err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "db error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch torrents", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Sync live stats from qBittorrent
	if len(torrents) > 0 {
		hashes := make([]string, len(torrents))
		for i, t := range torrents {
			hashes[i] = t.Hash
		}

		liveData, err := services.QBClient.GetTorrents(hashes)
		if err != nil {
			slog.WarnContext(ctx, "could not sync qbt stats", "error", err)
		} else {
			liveMap := make(map[string]services.QBTorrent, len(liveData))
			for _, qt := range liveData {
				liveMap[qt.Hash] = qt
			}
			for i, t := range torrents {
				if qt, ok := liveMap[t.Hash]; ok {
					torrents[i].Progress = qt.Progress
					torrents[i].Downloaded = qt.Downloaded
					torrents[i].Size = qt.Size
					torrents[i].Status = mapQBState(qt.State)
				}
			}
			// Persist updated stats async (best-effort)
			go func() {
				for _, t := range torrents {
					db.DB.Model(&t).Updates(map[string]interface{}{
						"progress":   t.Progress,
						"downloaded": t.Downloaded,
						"size":       t.Size,
						"status":     t.Status,
						"updated_at": time.Now(),
					})
				}
			}()
		}
	}

	c.JSON(http.StatusOK, gin.H{"torrents": torrents, "count": len(torrents)})
}

// AddMagnet handles POST /api/v1/torrents/magnet
func AddMagnet(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.add_magnet")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	var req struct {
		MagnetURL string `json:"magnet_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	if err := checkStorageQuota(ctx, userID); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	savePath := services.UserStoragePath(userID)

	if err := services.QBClient.AddMagnet(req.MagnetURL, savePath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "qbt error")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to add magnet", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Extract hash from magnet URI (urn:btih:<hash>)
	hash := extractMagnetHash(req.MagnetURL)
	name := extractMagnetName(req.MagnetURL)
	if name == "" {
		name = hash
	}

	torrent := &models.Torrent{
		UserID:   uuid.MustParse(userID),
		Hash:     hash,
		Name:     name,
		SavePath: savePath,
		Status:   models.StatusQueued,
		AddedAt:  time.Now(),
	}
	if err := db.DB.Create(torrent).Error; err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record torrent", "trace_id": observability.TraceID(ctx)})
		return
	}

	observability.RecordTorrentAdded(ctx, userID)
	slog.InfoContext(ctx, "magnet added", "user_id", userID, "hash", hash)
	c.JSON(http.StatusCreated, gin.H{"torrent": torrent})
}

// AddTorrentFile handles POST /api/v1/torrents/file
func AddTorrentFile(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.add_file")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	if err := checkStorageQuota(ctx, userID); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	fileHeader, err := c.FormFile("torrent")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "torrent file required", "trace_id": observability.TraceID(ctx)})
		return
	}

	if fileHeader.Size > 10*1024*1024 { // 10 MB sanity limit for .torrent files
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "torrent file too large"})
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}
	defer f.Close()

	fileBytes, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	savePath := services.UserStoragePath(userID)
	if err := services.QBClient.AddTorrentFile(fileBytes, fileHeader.Filename, savePath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "qbt error")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to add torrent", "trace_id": observability.TraceID(ctx)})
		return
	}

	torrent := &models.Torrent{
		UserID:   uuid.MustParse(userID),
		Hash:     "pending", // will be updated by sync
		Name:     fileHeader.Filename,
		SavePath: savePath,
		Status:   models.StatusQueued,
		AddedAt:  time.Now(),
	}
	if err := db.DB.Create(torrent).Error; err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record torrent", "trace_id": observability.TraceID(ctx)})
		return
	}

	observability.RecordTorrentAdded(ctx, userID)
	slog.InfoContext(ctx, "torrent file added", "user_id", userID, "filename", fileHeader.Filename)
	c.JSON(http.StatusCreated, gin.H{"torrent": torrent})
}

// GetTorrent handles GET /api/v1/torrents/:id
func GetTorrent(c *gin.Context) {
	_, span := torrentTracer.Start(c.Request.Context(), "torrents.get")
	defer span.End()

	userID := middleware.GetUserID(c)
	torrentID := c.Param("id")
	span.SetAttributes(attribute.String("user.id", userID), attribute.String("torrent.id", torrentID))

	var torrent models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ?", torrentID, userID).First(&torrent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "torrent not found"})
		return
	}

	// Refresh from qBittorrent
	if qt, err := services.QBClient.GetTorrent(torrent.Hash); err == nil {
		torrent.Progress = qt.Progress
		torrent.Downloaded = qt.Downloaded
		torrent.Size = qt.Size
		torrent.Status = mapQBState(qt.State)
	}

	c.JSON(http.StatusOK, torrent)
}

// DeleteTorrent handles DELETE /api/v1/torrents/:id
func DeleteTorrent(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.delete")
	defer span.End()

	userID := middleware.GetUserID(c)
	torrentID := c.Param("id")
	deleteFiles := c.Query("delete_files") == "true"
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("torrent.id", torrentID),
		attribute.Bool("delete_files", deleteFiles),
	)

	var torrent models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ?", torrentID, userID).First(&torrent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "torrent not found"})
		return
	}

	if err := services.QBClient.DeleteTorrent(torrent.Hash, deleteFiles); err != nil {
		slog.WarnContext(ctx, "qbt delete warning", "hash", torrent.Hash, "error", err)
	}

	if err := db.DB.Delete(&torrent).Error; err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete torrent record", "trace_id": observability.TraceID(ctx)})
		return
	}

	observability.RecordTorrentDeleted(ctx, userID)
	slog.InfoContext(ctx, "torrent deleted", "user_id", userID, "hash", torrent.Hash, "delete_files", deleteFiles)
	c.JSON(http.StatusOK, gin.H{"message": "torrent deleted"})
}

// PauseTorrent handles POST /api/v1/torrents/:id/pause
func PauseTorrent(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.pause")
	defer span.End()

	userID := middleware.GetUserID(c)
	torrentID := c.Param("id")

	var torrent models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ?", torrentID, userID).First(&torrent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "torrent not found"})
		return
	}

	if err := services.QBClient.PauseTorrent(torrent.Hash); err != nil {
		span.RecordError(err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to pause torrent", "trace_id": observability.TraceID(ctx)})
		return
	}

	db.DB.Model(&torrent).Update("status", models.StatusPaused)
	c.JSON(http.StatusOK, gin.H{"message": "torrent paused"})
}

// ResumeTorrent handles POST /api/v1/torrents/:id/resume
func ResumeTorrent(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.resume")
	defer span.End()

	userID := middleware.GetUserID(c)
	torrentID := c.Param("id")

	var torrent models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ?", torrentID, userID).First(&torrent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "torrent not found"})
		return
	}

	if err := services.QBClient.ResumeTorrent(torrent.Hash); err != nil {
		span.RecordError(err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to resume torrent", "trace_id": observability.TraceID(ctx)})
		return
	}

	db.DB.Model(&torrent).Update("status", models.StatusDownloading)
	c.JSON(http.StatusOK, gin.H{"message": "torrent resumed"})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func checkStorageQuota(ctx context.Context, userID string) error {
	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		return fmt.Errorf("user not found")
	}
	used, _ := services.DirSize(services.UserStoragePath(userID))
	if used >= user.StorageQuota {
		return fmt.Errorf("storage quota exceeded (%.2f GB used)", float64(used)/1e9)
	}
	return nil
}

func mapQBState(state string) models.TorrentStatus {
	switch state {
	case "downloading", "metaDL", "checkingDL":
		return models.StatusDownloading
	case "uploading", "stalledUP", "queuedUP", "forcedUP":
		return models.StatusSeeding
	case "pausedDL", "pausedUP":
		return models.StatusPaused
	case "error", "missingFiles", "unknown":
		return models.StatusError
	case "queuedDL":
		return models.StatusQueued
	default:
		if state != "" {
			return models.StatusCompleted
		}
		return models.StatusQueued
	}
}

func extractMagnetHash(magnet string) string {
	// magnet:?xt=urn:btih:<hash>&...
	const prefix = "urn:btih:"
	idx := len(magnet)
	start := -1
	for i := 0; i < len(magnet)-len(prefix); i++ {
		if magnet[i:i+len(prefix)] == prefix {
			start = i + len(prefix)
			break
		}
	}
	if start == -1 {
		return uuid.New().String()
	}
	for i := start; i < len(magnet); i++ {
		if magnet[i] == '&' || magnet[i] == ' ' {
			idx = i
			break
		}
	}
	if idx == len(magnet) {
		return magnet[start:]
	}
	return magnet[start:idx]
}

func extractMagnetName(magnet string) string {
	const prefix = "dn="
	start := -1
	for i := 0; i < len(magnet)-len(prefix); i++ {
		if magnet[i:i+len(prefix)] == prefix {
			start = i + len(prefix)
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(magnet)
	for i := start; i < len(magnet); i++ {
		if magnet[i] == '&' {
			end = i
			break
		}
	}
	return magnet[start:end]
}

package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

var torrentTracer = otel.Tracer("openseedr/handlers/torrents")

// TorrentResponse wraps the DB model with live-only fields from qBittorrent
// that we don't want to persist (speeds, ETA, peer counts).
type TorrentResponse struct {
	models.Torrent
	DownloadSpeed int64 `json:"download_speed"`
	UploadSpeed   int64 `json:"upload_speed"`
	Eta           int64 `json:"eta"`
	NumSeeds      int   `json:"num_seeds"`
	NumLeechs     int   `json:"num_leechs"`
}

// ListTorrents handles GET /api/v1/torrents
func ListTorrents(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.list")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	var torrents []models.Torrent
	if err := db.DB.Where("user_id = ? AND status != ?", userID, models.StatusWishlist).Order("created_at desc").Find(&torrents).Error; err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "db error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch torrents", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Sync live stats from qBittorrent
	responses := make([]TorrentResponse, len(torrents))
	for i, t := range torrents {
		responses[i] = TorrentResponse{Torrent: t}
	}

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
				liveStatus := mapQBState(qt.State)
				responses[i].Progress = qt.Progress
				responses[i].Downloaded = qt.Downloaded
				responses[i].Size = qt.Size

				// Grace-period guard: if the user just issued a pause or resume
				// command, qBittorrent may not have applied it yet by the time
				// the next poll fires. Keep the DB status for up to 10 seconds
				// when there is a pause-vs-active conflict, so the UI doesn't
				// flicker back to the previous state on the very next poll.
				recentlyUpdated := time.Since(t.UpdatedAt) < 10*time.Second
				pauseConflict := (t.Status == models.StatusPaused && liveStatus != models.StatusPaused) ||
					(t.Status != models.StatusPaused && liveStatus == models.StatusPaused)
				if recentlyUpdated && pauseConflict {
					responses[i].Status = t.Status
				} else {
					responses[i].Status = liveStatus
				}

				responses[i].DownloadSpeed = qt.DownloadSpeed
				responses[i].UploadSpeed = qt.UploadSpeed
				responses[i].Eta = qt.Eta
				responses[i].NumSeeds = qt.NumSeeds
				responses[i].NumLeechs = qt.NumLeechs
			}
		}
		// Persist updated stats async (best-effort).
		// Use UpdateColumns (not Updates) so GORM does not auto-reset updated_at
		// on every poll — updated_at must only change on real user actions
		// (pause, resume, add) so the grace-period guard above works correctly.
		// Skip rows where nothing changed — most polls have no changes for
		// completed/seeding/paused torrents, avoiding N unnecessary DB round-trips.
		go func() {
			for i, r := range responses {
				orig := torrents[i]
				if r.Progress == orig.Progress && r.Downloaded == orig.Downloaded &&
					r.Size == orig.Size && r.Status == orig.Status {
					continue // nothing changed — skip the UPDATE
				}
				db.DB.Model(&r.Torrent).UpdateColumns(map[string]interface{}{
					"progress":   r.Progress,
					"downloaded": r.Downloaded,
					"size":       r.Size,
					"status":     r.Status,
				})
			}
		}()
		}
	}

	c.JSON(http.StatusOK, gin.H{"torrents": responses, "count": len(responses)})
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

	hash := extractMagnetHash(req.MagnetURL)
	name := extractMagnetName(req.MagnetURL)
	if name == "" {
		name = hash
	}
	savePath := services.UserStoragePath(userID)

	// Fetch the user once — used for quota check and torrent creation below.
	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found", "trace_id": observability.TraceID(ctx)})
		return
	}

	// If quota is exceeded, save to wishlist instead of returning 403.
	if err := checkStorageQuota(ctx, &user); err != nil {
		torrent, dbErr := upsertWishlistMagnet(userID, hash, name, savePath, req.MagnetURL)
		if dbErr != nil {
			span.RecordError(dbErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save to wishlist", "trace_id": observability.TraceID(ctx)})
			return
		}
		slog.InfoContext(ctx, "magnet wishlisted (quota full)", "user_id", userID, "hash", hash)
		c.JSON(http.StatusAccepted, gin.H{
			"wishlisted": true,
			"message":    "Storage quota is full. Torrent has been saved to your wishlist and will be added automatically once space is available.",
			"torrent":    torrent,
		})
		return
	}

	if err := services.QBClient.AddMagnet(req.MagnetURL, savePath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "qbt error")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to add magnet", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Restore or create: use Unscoped so we can see soft-deleted records too.
	// If the user previously had this torrent (active, errored, deleted, or
	// wishlisted), restore it to queued.
	var torrent models.Torrent
	existing := db.DB.Unscoped().
		Where("user_id = ? AND hash = ?", userID, hash).
		First(&torrent)

	if existing.Error == nil {
		now := time.Now()
		if err := db.DB.Unscoped().Model(&torrent).Updates(map[string]interface{}{
			"name":         name,
			"save_path":    savePath,
			"status":       models.StatusQueued,
			"progress":     0,
			"downloaded":   0,
			"size":         0,
			"magnet_url":   "",
			"torrent_data": nil,
			"added_at":     now,
			"updated_at":   now,
			"deleted_at":   nil,
		}).Error; err != nil {
			span.RecordError(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record torrent", "trace_id": observability.TraceID(ctx)})
			return
		}
	} else {
		torrent = models.Torrent{
			UserID:   uuid.MustParse(userID),
			Hash:     hash,
			Name:     name,
			SavePath: savePath,
			Status:   models.StatusQueued,
			AddedAt:  time.Now(),
		}
		if err := db.DB.Create(&torrent).Error; err != nil {
			span.RecordError(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record torrent", "trace_id": observability.TraceID(ctx)})
			return
		}
	}

	observability.RecordTorrentAdded(ctx, userID)
	slog.InfoContext(ctx, "magnet added", "user_id", userID, "hash", hash)
	db.DB.Unscoped().First(&torrent, torrent.ID)
	c.JSON(http.StatusCreated, gin.H{"torrent": torrent})
}

// upsertWishlistMagnet creates or updates a wishlist record for a magnet link.
func upsertWishlistMagnet(userID, hash, name, savePath, magnetURL string) (models.Torrent, error) {
	var torrent models.Torrent
	existing := db.DB.Unscoped().Where("user_id = ? AND hash = ?", userID, hash).First(&torrent)
	now := time.Now()
	if existing.Error == nil {
		err := db.DB.Unscoped().Model(&torrent).Updates(map[string]interface{}{
			"name":         name,
			"save_path":    savePath,
			"status":       models.StatusWishlist,
			"magnet_url":   magnetURL,
			"torrent_data": nil,
			"progress":     0,
			"downloaded":   0,
			"size":         0,
			"added_at":     now,
			"updated_at":   now,
			"deleted_at":   nil,
		})
		return torrent, err.Error
	}
	torrent = models.Torrent{
		UserID:    uuid.MustParse(userID),
		Hash:      hash,
		Name:      name,
		SavePath:  savePath,
		Status:    models.StatusWishlist,
		MagnetURL: magnetURL,
		AddedAt:   now,
	}
	return torrent, db.DB.Create(&torrent).Error
}

// AddTorrentFile handles POST /api/v1/torrents/file
func AddTorrentFile(c *gin.Context) {
	ctx, span := torrentTracer.Start(c.Request.Context(), "torrents.add_file")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

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

	// Extract the real info-hash from the .torrent bytes so we can match this
	// record against qBittorrent's live data later.  Fall back to a unique
	// placeholder only if parsing fails (malformed file).
	infoHash, hashErr := services.ExtractInfoHash(fileBytes)
	if hashErr != nil {
		slog.WarnContext(ctx, "could not extract info hash", "filename", fileHeader.Filename, "error", hashErr)
		infoHash = "pending-" + uuid.New().String()
	}

	// ── Derive a human-readable name from the torrent metadata ────────────────
	// Strip the .torrent extension from the filename as a reasonable default.
	torrentName := fileHeader.Filename
	if len(torrentName) > 8 && torrentName[len(torrentName)-8:] == ".torrent" {
		torrentName = torrentName[:len(torrentName)-8]
	}

	// Fetch the user once — used for quota check and torrent creation below.
	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		span.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found", "trace_id": observability.TraceID(ctx)})
		return
	}

	// If quota is exceeded, save the raw .torrent bytes to the wishlist so
	// they can be submitted to qBittorrent once space becomes available.
	if err := checkStorageQuota(ctx, &user); err != nil {
		torrent := &models.Torrent{
			UserID:      uuid.MustParse(userID),
			Hash:        infoHash,
			Name:        torrentName,
			SavePath:    savePath,
			Status:      models.StatusWishlist,
			TorrentData: fileBytes,
			AddedAt:     time.Now(),
		}
		if dbErr := db.DB.Create(torrent).Error; dbErr != nil {
			span.RecordError(dbErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save to wishlist", "trace_id": observability.TraceID(ctx)})
			return
		}
		slog.InfoContext(ctx, "torrent file wishlisted (quota full)", "user_id", userID, "filename", fileHeader.Filename)
		c.JSON(http.StatusAccepted, gin.H{
			"wishlisted": true,
			"message":    "Storage quota is full. Torrent has been saved to your wishlist and will be added automatically once space is available.",
			"torrent":    torrent,
		})
		return
	}

	if err := services.QBClient.AddTorrentFile(fileBytes, fileHeader.Filename, savePath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "qbt error")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to add torrent", "trace_id": observability.TraceID(ctx)})
		return
	}

	// Restore or create the DB record using the real hash so that the live-
	// stats sync (which also uses the hash) works immediately.
	var torrent models.Torrent
	existing := db.DB.Unscoped().Where("user_id = ? AND hash = ?", userID, infoHash).First(&torrent)
	now := time.Now()
	if existing.Error == nil {
		if err := db.DB.Unscoped().Model(&torrent).Updates(map[string]interface{}{
			"name":         torrentName,
			"save_path":    savePath,
			"status":       models.StatusQueued,
			"progress":     0,
			"downloaded":   0,
			"size":         0,
			"torrent_data": nil,
			"added_at":     now,
			"updated_at":   now,
			"deleted_at":   nil,
		}).Error; err != nil {
			span.RecordError(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record torrent", "trace_id": observability.TraceID(ctx)})
			return
		}
	} else {
		torrent = models.Torrent{
			UserID:   uuid.MustParse(userID),
			Hash:     infoHash,
			Name:     torrentName,
			SavePath: savePath,
			Status:   models.StatusQueued,
			AddedAt:  now,
		}
		if err := db.DB.Create(&torrent).Error; err != nil {
			span.RecordError(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record torrent", "trace_id": observability.TraceID(ctx)})
			return
		}
	}

	observability.RecordTorrentAdded(ctx, userID)
	slog.InfoContext(ctx, "torrent file added", "user_id", userID, "hash", infoHash, "filename", fileHeader.Filename)
	db.DB.Unscoped().First(&torrent, torrent.ID)
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
	response := TorrentResponse{Torrent: torrent}
	if qt, err := services.QBClient.GetTorrent(torrent.Hash); err == nil {
		response.Progress = qt.Progress
		response.Downloaded = qt.Downloaded
		response.Size = qt.Size
		response.Status = mapQBState(qt.State)
		response.DownloadSpeed = qt.DownloadSpeed
		response.UploadSpeed = qt.UploadSpeed
		response.Eta = qt.Eta
		response.NumSeeds = qt.NumSeeds
		response.NumLeechs = qt.NumLeechs
	}

	c.JSON(http.StatusOK, response)
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

// GetTorrentFiles handles GET /api/v1/torrents/:id/files
func GetTorrentFiles(c *gin.Context) {
	_, span := torrentTracer.Start(c.Request.Context(), "torrents.files")
	defer span.End()

	userID := middleware.GetUserID(c)
	torrentID := c.Param("id")
	span.SetAttributes(attribute.String("user.id", userID), attribute.String("torrent.id", torrentID))

	var torrent models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ?", torrentID, userID).First(&torrent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "torrent not found"})
		return
	}

	files, err := services.QBClient.GetTorrentFiles(torrent.Hash)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch torrent files"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

// ── helpers ──────────────────────────────────────────────────────────────────

// checkStorageQuota checks whether the user has exceeded their storage quota.
// The caller must pass the already-fetched user so we avoid a redundant DB round-trip.
func checkStorageQuota(ctx context.Context, user *models.User) error {
	used, _ := services.DirSize(services.UserStoragePath(user.ID.String()))
	if used >= user.StorageQuota {
		return fmt.Errorf("storage quota exceeded (%.2f GB used)", float64(used)/1e9)
	}
	return nil
}

func mapQBState(state string) models.TorrentStatus {
	switch state {
	// ── Actively downloading ───────────────────────────────────────────────────
	case "downloading",        // transferring data
		"metaDL",              // fetching metadata (magnet)
		"checkingDL",          // hash-checking partially-downloaded data
		"stalledDL",           // downloading but no active peer connections
		"allocating":          // allocating disk space before download starts
		return models.StatusDownloading

	// ── Seeding (upload only) ─────────────────────────────────────────────────
	case "uploading",          // actively uploading
		"stalledUP",           // seeding but no active peer connections
		"queuedUP",            // queued, waiting for upload slot
		"forcedUP",            // forced upload (ignores queue limit)
		"checkingUP",          // hash-checking completed data before seeding
		"moving":              // relocating completed data to a new path
		return models.StatusSeeding

	// ── Paused ────────────────────────────────────────────────────────────────
	// qBittorrent v4: pausedDL / pausedUP
	// qBittorrent v5: stoppedDL / stoppedUP  (renamed in v5)
	case "pausedDL", "pausedUP", "stoppedDL", "stoppedUP":
		return models.StatusPaused

	// ── Error ─────────────────────────────────────────────────────────────────
	case "error", "missingFiles":
		return models.StatusError

	// ── Queued / startup ──────────────────────────────────────────────────────
	case "queuedDL",           // queued, waiting for download slot
		"checkingResumeData",  // verifying resume data on qBT startup
		"unknown":             // qBittorrent internal unknown state
		return models.StatusQueued

	// ── Anything else ─────────────────────────────────────────────────────────
	// Default to queued (not completed) so a new torrent never jumps
	// unexpectedly to a terminal state when qBittorrent adds a new state.
	default:
		return models.StatusQueued
	}
}

func extractMagnetHash(magnet string) string {
	u, err := url.Parse(magnet)
	if err == nil {
		if xt := u.Query().Get("xt"); strings.HasPrefix(xt, "urn:btih:") {
			// qBittorrent stores and returns hashes in lowercase hex.
			// Magnet links may carry the hash as uppercase hex or base32;
			// normalise to lowercase so the live-stats lookup always matches.
			return strings.ToLower(strings.TrimPrefix(xt, "urn:btih:"))
		}
	}
	return uuid.New().String()
}

func extractMagnetName(magnet string) string {
	u, err := url.Parse(magnet)
	if err != nil {
		return ""
	}
	// url.Query().Get automatically URL-decodes the value, turning
	// '+' → ' ' and '%XX' sequences into their proper characters.
	return u.Query().Get("dn")
}

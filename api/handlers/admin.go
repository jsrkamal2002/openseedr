package handlers

import (
	"log/slog"
	"net/http"

	"github.com/openseedr/api/db"
	"github.com/openseedr/api/models"
	"github.com/openseedr/api/observability"
	"github.com/openseedr/api/services"
	"github.com/gin-gonic/gin"
)

// ── Admin: Users ─────────────────────────────────────────────────────────────

// AdminListUsers handles GET /api/v1/admin/users
// Returns a paginated list of all users.
func AdminListUsers(c *gin.Context) {
	var users []models.User
	if err := db.DB.Order("created_at desc").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to fetch users",
			"trace_id": observability.TraceID(c.Request.Context()),
		})
		return
	}
	c.JSON(http.StatusOK, users)
}

// AdminGetUser handles GET /api/v1/admin/users/:id
func AdminGetUser(c *gin.Context) {
	id := c.Param("id")
	var user models.User
	if err := db.DB.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// AdminUpdateUser handles PATCH /api/v1/admin/users/:id
// Allows updating: storage_quota, download_limit, upload_limit, is_admin, is_active.
func AdminUpdateUser(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		StorageQuota  *int64 `json:"storage_quota"`
		DownloadLimit *int64 `json:"download_limit"` // bytes/sec; 0 = unlimited
		UploadLimit   *int64 `json:"upload_limit"`   // bytes/sec; 0 = unlimited
		IsAdmin       *bool  `json:"is_admin"`
		IsActive      *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := db.DB.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	updates := map[string]interface{}{}
	if req.StorageQuota != nil {
		updates["storage_quota"] = *req.StorageQuota
	}
	if req.DownloadLimit != nil {
		updates["download_limit"] = *req.DownloadLimit
	}
	if req.UploadLimit != nil {
		updates["upload_limit"] = *req.UploadLimit
	}
	if req.IsAdmin != nil {
		updates["is_admin"] = *req.IsAdmin
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	if err := db.DB.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to update user",
			"trace_id": observability.TraceID(c.Request.Context()),
		})
		return
	}

	// Reload to return updated record
	db.DB.First(&user, "id = ?", id)
	slog.Info("admin updated user", "target_user_id", id)
	c.JSON(http.StatusOK, user)
}

// AdminDeleteUser handles DELETE /api/v1/admin/users/:id
// Soft-deletes the user via GORM's DeletedAt.
func AdminDeleteUser(c *gin.Context) {
	id := c.Param("id")
	var user models.User
	if err := db.DB.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := db.DB.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to delete user",
			"trace_id": observability.TraceID(c.Request.Context()),
		})
		return
	}

	slog.Info("admin deleted user", "target_user_id", id)
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

// ── Admin: System stats ───────────────────────────────────────────────────────

// AdminStats handles GET /api/v1/admin/stats
// Returns aggregate system statistics.
func AdminStats(c *gin.Context) {
	ctx := c.Request.Context()

	var totalUsers int64
	db.DB.Model(&models.User{}).Count(&totalUsers)

	var activeUsers int64
	db.DB.Model(&models.User{}).Where("is_active = true").Count(&activeUsers)

	var totalTorrents int64
	db.DB.Model(&models.Torrent{}).Count(&totalTorrents)

	// Sum storage used across all users
	type storageResult struct {
		Total int64
	}
	var sr storageResult
	db.DB.Model(&models.User{}).Select("COALESCE(SUM(storage_used), 0) as total").Scan(&sr)

	// Sum storage quota across all users
	var sqr storageResult
	db.DB.Model(&models.User{}).Select("COALESCE(SUM(storage_quota), 0) as total").Scan(&sqr)

	// Live qBittorrent stats
	qbtStats := map[string]interface{}{}
	if torrents, err := services.QBClient.GetTorrents(nil); err == nil {
		downloading, seeding, paused := 0, 0, 0
		for _, t := range torrents {
			switch t.State {
			case "downloading", "metaDL", "checkingDL", "forcedDL":
				downloading++
			case "uploading", "forcedUP", "stalledUP":
				seeding++
			case "pausedDL", "pausedUP":
				paused++
			}
		}
		qbtStats["total"] = len(torrents)
		qbtStats["downloading"] = downloading
		qbtStats["seeding"] = seeding
		qbtStats["paused"] = paused
	} else {
		slog.WarnContext(ctx, "admin stats: failed to fetch qbt torrents", "error", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"users": gin.H{
			"total":  totalUsers,
			"active": activeUsers,
		},
		"torrents": gin.H{
			"db_total": totalTorrents,
			"live":     qbtStats,
		},
		"storage": gin.H{
			"used_bytes":  sr.Total,
			"quota_bytes": sqr.Total,
		},
	})
}

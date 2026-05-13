package handlers

import (
	"context"
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

var wishlistTracer = otel.Tracer("openseedr/handlers/wishlist")

// ListWishlist handles GET /api/v1/wishlist
// Returns all torrents in the user's wishlist ordered by when they were queued.
func ListWishlist(c *gin.Context) {
	_, span := wishlistTracer.Start(c.Request.Context(), "wishlist.list")
	defer span.End()

	userID := middleware.GetUserID(c)
	span.SetAttributes(attribute.String("user.id", userID))

	var items []models.Torrent
	if err := db.DB.Where("user_id = ? AND status = ?", userID, models.StatusWishlist).
		Order("added_at asc").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch wishlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"wishlist": items, "count": len(items)})
}

// RemoveWishlistItem handles DELETE /api/v1/wishlist/:id
// Removes an item from the wishlist without ever downloading it.
func RemoveWishlistItem(c *gin.Context) {
	_, span := wishlistTracer.Start(c.Request.Context(), "wishlist.remove")
	defer span.End()

	userID := middleware.GetUserID(c)
	itemID := c.Param("id")

	var item models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ? AND status = ?", itemID, userID, models.StatusWishlist).
		First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "wishlist item not found"})
		return
	}

	if err := db.DB.Delete(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove wishlist item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "removed from wishlist"})
}

// PromoteWishlistItem handles POST /api/v1/wishlist/:id/promote
// Attempts to start the download for a single wishlist item immediately.
// Returns 403 if the quota is still exceeded, 200 on success.
func PromoteWishlistItem(c *gin.Context) {
	ctx, span := wishlistTracer.Start(c.Request.Context(), "wishlist.promote")
	defer span.End()

	userID := middleware.GetUserID(c)
	itemID := c.Param("id")
	span.SetAttributes(attribute.String("user.id", userID), attribute.String("item.id", itemID))

	var item models.Torrent
	if err := db.DB.Where("id = ? AND user_id = ? AND status = ?", itemID, userID, models.StatusWishlist).
		First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "wishlist item not found"})
		return
	}

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	if err := checkStorageQuota(ctx, &user); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	if err := promoteItem(&item); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "promote failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "trace_id": observability.TraceID(ctx)})
		return
	}

	observability.RecordTorrentAdded(ctx, userID)
	slog.InfoContext(ctx, "wishlist item promoted", "user_id", userID, "id", itemID)
	c.JSON(http.StatusOK, gin.H{"torrent": item})
}

// promoteItem submits a wishlist torrent to qBittorrent and updates its DB record.
// Called by PromoteWishlistItem (HTTP handler) and AutoPromoteWishlist (background worker).
func promoteItem(item *models.Torrent) error {
	savePath := item.SavePath

	if item.MagnetURL != "" {
		if err := services.QBClient.AddMagnet(item.MagnetURL, savePath); err != nil {
			return err
		}
		return db.DB.Model(item).Updates(map[string]interface{}{
			"status":       models.StatusQueued,
			"magnet_url":   "",
			"progress":     0,
			"downloaded":   0,
			"size":         0,
			"updated_at":   time.Now(),
		}).Error
	}

	if len(item.TorrentData) > 0 {
		if err := services.QBClient.AddTorrentFile(item.TorrentData, item.Name, savePath); err != nil {
			return err
		}
		return db.DB.Model(item).Updates(map[string]interface{}{
			"status":       models.StatusQueued,
			"torrent_data": nil,
			"hash":         "pending-" + uuid.New().String(),
			"progress":     0,
			"downloaded":   0,
			"size":         0,
			"updated_at":   time.Now(),
		}).Error
	}

	// Nothing to submit (should not happen in practice)
	return nil
}

// AutoPromoteWishlist is intended to be run as a background goroutine.
// Every 5 minutes it checks all users that have wishlist items and promotes
// them in FIFO order until the user's quota is full again.
func AutoPromoteWishlist() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		promoteAll()
	}
}

func promoteAll() {
	ctx := context.Background()

	// Find distinct users who have wishlist items
	var userIDs []string
	if err := db.DB.Model(&models.Torrent{}).
		Where("status = ?", models.StatusWishlist).
		Distinct("user_id").
		Pluck("user_id", &userIDs).Error; err != nil {
		slog.ErrorContext(ctx, "wishlist auto-promote: failed to query users", "error", err)
		return
	}

	for _, uid := range userIDs {
		var user models.User
		if err := db.DB.First(&user, "id = ?", uid).Error; err != nil {
			continue
		}

		used, err := services.DirSize(services.UserStoragePath(uid))
		if err != nil || used >= user.StorageQuota {
			continue // no space yet
		}

		var items []models.Torrent
		if err := db.DB.Where("user_id = ? AND status = ?", uid, models.StatusWishlist).
			Order("added_at asc").Find(&items).Error; err != nil {
			continue
		}

		for i := range items {
			// Re-check quota before each item (previous promotes consume space)
			used, _ = services.DirSize(services.UserStoragePath(uid))
			if used >= user.StorageQuota {
				break
			}
			if err := promoteItem(&items[i]); err != nil {
				slog.ErrorContext(ctx, "wishlist auto-promote: failed to promote item",
					"user_id", uid, "id", items[i].ID, "error", err)
				continue
			}
			slog.InfoContext(ctx, "wishlist auto-promote: promoted item",
				"user_id", uid, "id", items[i].ID, "name", items[i].Name)
		}
	}
}


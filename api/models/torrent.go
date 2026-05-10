package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TorrentStatus string

const (
	StatusQueued      TorrentStatus = "queued"
	StatusDownloading TorrentStatus = "downloading"
	StatusSeeding     TorrentStatus = "seeding"
	StatusPaused      TorrentStatus = "paused"
	StatusCompleted   TorrentStatus = "completed"
	StatusError       TorrentStatus = "error"
	// StatusWishlist means the torrent was requested but could not be started
	// immediately because the user's storage quota was full.  It will be
	// promoted automatically (or manually) once space becomes available.
	StatusWishlist TorrentStatus = "wishlist"
)

type Torrent struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid;index;not null;uniqueIndex:idx_torrents_user_hash" json:"user_id"`
	Hash         string         `gorm:"not null;uniqueIndex:idx_torrents_user_hash" json:"hash"`
	Name         string         `gorm:"not null" json:"name"`
	Size         int64          `gorm:"default:0" json:"size"`
	Downloaded   int64          `gorm:"default:0" json:"downloaded"`
	Progress     float64        `gorm:"default:0" json:"progress"`
	Status       TorrentStatus  `gorm:"default:'queued'" json:"status"`
	SavePath     string         `gorm:"not null" json:"save_path"`
	AddedAt      time.Time      `json:"added_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	// Wishlist support: store the original source so it can be re-submitted
	// to qBittorrent once storage quota is freed.
	MagnetURL   string `gorm:"default:''" json:"magnet_url,omitempty"`
	TorrentData []byte `gorm:"type:bytea" json:"-"` // raw .torrent bytes; only set for file-based wishlist items
}

func (t *Torrent) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

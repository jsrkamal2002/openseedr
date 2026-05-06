package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string     `gorm:"uniqueIndex;not null" json:"email"`
	Username     string     `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"" json:"-"`
	Provider     string     `gorm:"default:'local'" json:"provider"` // local, google, github
	ProviderID   string     `gorm:"" json:"provider_id,omitempty"`
	AvatarURL    string     `gorm:"" json:"avatar_url,omitempty"`
	StorageQuota int64      `gorm:"default:10737418240" json:"storage_quota"` // 10 GB default
	StorageUsed  int64      `gorm:"default:0" json:"storage_used"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

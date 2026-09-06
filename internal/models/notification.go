package models

import (
	"time"

	"gorm.io/gorm"
)

type Notification struct {
	gorm.Model
	UserID          uint   `gorm:"not null;index:idx_notifications_user_read"`
	Type            string `gorm:"size:64;not null;index"`
	Title           string `gorm:"size:160;not null"`
	Message         string `gorm:"size:500;not null"`
	RelatedOrderID  uint   `gorm:"not null;index"`
	ShippingEventID string `gorm:"size:64;not null;uniqueIndex"`
	IsRead          bool   `gorm:"not null;default:false;index:idx_notifications_user_read"`
	ReadAt          *time.Time
}

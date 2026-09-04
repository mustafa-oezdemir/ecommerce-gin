package models

import "time"

type RecoveryCode struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UserID    uint       `gorm:"not null;uniqueIndex:idx_recovery_codes_user_hash"`
	CodeHash  []byte     `gorm:"type:binary(32);not null;uniqueIndex:idx_recovery_codes_user_hash"`
	UsedAt    *time.Time `gorm:"index:idx_recovery_codes_user_unused"`
}

type EmailChangeRequest struct {
	ID           uint `gorm:"primaryKey"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	UserID       uint      `gorm:"not null;uniqueIndex"`
	PendingEmail string    `gorm:"size:254;not null;index"`
	CodeHash     []byte    `gorm:"type:binary(32);not null"`
	ExpiresAt    time.Time `gorm:"not null;index"`
	Attempts     uint      `gorm:"not null;default:0"`
}

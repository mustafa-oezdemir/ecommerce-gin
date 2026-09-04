package models

import "time"

type ProductReview struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    uint `gorm:"not null;uniqueIndex:idx_product_reviews_user_product"`
	User      User
	ProductID uint   `gorm:"not null;uniqueIndex:idx_product_reviews_user_product"`
	Rating    uint8  `gorm:"not null"`
	Title     string `gorm:"size:150;not null"`
	Body      string `gorm:"type:text;not null"`
}

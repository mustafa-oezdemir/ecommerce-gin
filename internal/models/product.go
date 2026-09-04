package models

import "gorm.io/gorm"

type Product struct {
	gorm.Model
	Name          string          `gorm:"size:200;not null;index"`
	Description   string          `gorm:"type:text"`
	ImageFilename string          `gorm:"size:255"`
	Images        []ProductImage  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Reviews       []ProductReview `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	PriceCents    int64           `gorm:"not null"`
	Stock         int             `gorm:"not null;default:0"`
	Active        bool            `gorm:"not null;default:true;index"`
	CategoryID    *uint           `gorm:"index"`
	Category      *Category       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

type ProductImage struct {
	gorm.Model
	ProductID uint   `gorm:"not null;index;uniqueIndex:idx_product_images_product_position"`
	Filename  string `gorm:"size:255;not null;uniqueIndex"`
	Position  int64  `gorm:"not null;uniqueIndex:idx_product_images_product_position"`
}

type ProductReview struct {
	gorm.Model
	ProductID uint   `gorm:"not null;uniqueIndex:idx_product_reviews_product_user"`
	UserID    uint   `gorm:"not null;uniqueIndex:idx_product_reviews_product_user"`
	User      User   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Rating    int    `gorm:"not null"`
	Comment   string `gorm:"type:text;not null"`
}

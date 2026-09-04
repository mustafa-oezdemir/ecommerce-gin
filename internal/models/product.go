package models

import (
	"sort"
	"time"

	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Name          string         `gorm:"size:200;not null;index"`
	Description   string         `gorm:"type:text"`
	ImageFilename string         `gorm:"size:255"`
	PriceCents    int64          `gorm:"not null"`
	Stock         int            `gorm:"not null;default:0"`
	Active        bool           `gorm:"not null;default:true;index"`
	CategoryID    *uint          `gorm:"index"`
	Category      *Category      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Images        []ProductImage `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type ProductImage struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	ProductID uint   `gorm:"not null;index:idx_product_images_order,priority:1"`
	Filename  string `gorm:"size:255;not null;uniqueIndex"`
	Position  uint   `gorm:"not null;default:0;index:idx_product_images_order,priority:2"`
}

// GalleryImages returns a copy with the cover image first and remaining images
// in their stable upload order.
func (product Product) GalleryImages() []ProductImage {
	images := append([]ProductImage(nil), product.Images...)
	sort.SliceStable(images, func(i, j int) bool {
		if images[i].Filename == product.ImageFilename {
			return true
		}
		if images[j].Filename == product.ImageFilename {
			return false
		}
		if images[i].Position == images[j].Position {
			return images[i].ID < images[j].ID
		}
		return images[i].Position < images[j].Position
	})
	if len(images) == 0 && product.ImageFilename != "" {
		images = append(images, ProductImage{Filename: product.ImageFilename})
	}
	return images
}

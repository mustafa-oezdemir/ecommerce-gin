package models

import (
	"sort"
	"time"

	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Name          string           `gorm:"size:200;not null;index"`
	Description   string           `gorm:"type:text"`
	ImageFilename string           `gorm:"size:255"`
	PriceCents    int64            `gorm:"not null"`
	Stock         int              `gorm:"not null;default:0"`
	Active        bool             `gorm:"not null;default:true;index"`
	CategoryID    *uint            `gorm:"index"`
	Category      *Category        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	BrandID       *uint            `gorm:"index"`
	Brand         *Brand           `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Images        []ProductImage   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Variants      []ProductVariant `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type Brand struct {
	gorm.Model
	Name     string    `gorm:"size:120;not null;uniqueIndex"`
	Products []Product `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

// ProductVariant holds the combination-specific discovery data. Keeping color
// and size on one row prevents filters from matching values from two variants.
type ProductVariant struct {
	gorm.Model
	ProductID    uint                      `gorm:"not null;index"`
	SKU          string                    `gorm:"size:100;not null;uniqueIndex"`
	Color        string                    `gorm:"size:80;index"`
	ClothingSize string                    `gorm:"size:30;index"`
	ShoeSize     string                    `gorm:"size:30;index"`
	Stock        int                       `gorm:"not null;default:0"`
	Active       bool                      `gorm:"not null;default:true;index"`
	Attributes   []ProductVariantAttribute `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type ProductVariantAttribute struct {
	gorm.Model
	ProductVariantID uint   `gorm:"not null;index:idx_variant_attribute,priority:1"`
	Name             string `gorm:"size:80;not null;index:idx_variant_attribute,priority:2"`
	Value            string `gorm:"size:120;not null;index:idx_variant_attribute,priority:3"`
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

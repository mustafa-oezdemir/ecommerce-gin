package models

import "gorm.io/gorm"

type Product struct {
	gorm.Model
	Name        string    `gorm:"size:200;not null;index"`
	Description string    `gorm:"type:text"`
	PriceCents  int64     `gorm:"not null"`
	Stock       int       `gorm:"not null;default:0"`
	Active      bool      `gorm:"not null;default:true;index"`
	CategoryID  *uint     `gorm:"index"`
	Category    *Category `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

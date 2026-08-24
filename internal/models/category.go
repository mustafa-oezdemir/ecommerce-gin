package models

import "gorm.io/gorm"

type Category struct {
	gorm.Model
	Name        string    `gorm:"size:100;uniqueIndex;not null"`
	Description string    `gorm:"type:text"`
	Products    []Product `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

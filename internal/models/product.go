package models

import "gorm.io/gorm"

type Product struct {
    gorm.Model
    Name        string  `gorm:"size:200"`
    Description string  `gorm:"type:text"`
    Price       float64 `gorm:"type:decimal(10,2)"`
    Stock       int
}

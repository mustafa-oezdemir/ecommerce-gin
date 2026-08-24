package models

import "gorm.io/gorm"

type Cart struct {
	gorm.Model
	UserID uint `gorm:"not null;uniqueIndex"`
	Items  []CartItem
}

type CartItem struct {
	gorm.Model
	CartID    uint `gorm:"not null;uniqueIndex:idx_cart_product"`
	ProductID uint `gorm:"not null;uniqueIndex:idx_cart_product"`
	Product   Product
	Quantity  int `gorm:"not null"`
}

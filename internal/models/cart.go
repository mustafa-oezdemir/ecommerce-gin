package models

import "gorm.io/gorm"

type Cart struct {
	gorm.Model
	UserID uint
	Items  []CartItem
}

type CartItem struct {
	gorm.Model
	CartID    uint
	ProductID uint
	Product   Product
	Quantity  int
}

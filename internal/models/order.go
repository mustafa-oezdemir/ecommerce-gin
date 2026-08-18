package models

import "gorm.io/gorm"

type Order struct {
    gorm.Model
    UserID uint
    User   User
    Status string `gorm:"size:50"`
    Items  []OrderItem
}

type OrderItem struct {
    gorm.Model
    OrderID   uint
    ProductID uint
    Product   Product
    Quantity  int
    Price     float64 `gorm:"type:decimal(10,2)"`
}

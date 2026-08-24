package models

import "gorm.io/gorm"

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusCompleted  OrderStatus = "completed"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

type Order struct {
	gorm.Model
	UserID uint
	User   User
	Status OrderStatus `gorm:"size:50;index"`
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

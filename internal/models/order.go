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

func AllowedOrderStatusTransitions(from OrderStatus) []OrderStatus {
	switch from {
	case OrderStatusPending:
		return []OrderStatus{OrderStatusProcessing, OrderStatusCancelled}
	case OrderStatusProcessing:
		return []OrderStatus{OrderStatusShipped, OrderStatusCancelled}
	case OrderStatusShipped:
		return []OrderStatus{OrderStatusCompleted}
	default:
		return nil
	}
}

func CanTransitionOrderStatus(from, to OrderStatus) bool {
	for _, allowed := range AllowedOrderStatusTransitions(from) {
		if allowed == to {
			return true
		}
	}
	return false
}

type Order struct {
	gorm.Model
	UserID     uint `gorm:"not null;index"`
	User       User
	Status     OrderStatus `gorm:"size:50;index;not null"`
	TotalCents int64       `gorm:"not null"`
	Items      []OrderItem
}

type OrderItem struct {
	gorm.Model
	OrderID        uint    `gorm:"not null;index"`
	ProductID      uint    `gorm:"not null"`
	Product        Product `gorm:"constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	ProductName    string  `gorm:"size:200;not null"`
	UnitPriceCents int64   `gorm:"not null"`
	Quantity       int     `gorm:"not null"`
	SubtotalCents  int64   `gorm:"not null"`
}

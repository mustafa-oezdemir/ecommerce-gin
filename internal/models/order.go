package models

import (
	"time"

	"gorm.io/gorm"
)

type OrderStatus string

const (
	OrderStatusPending        OrderStatus = "pending"
	OrderStatusPendingPayment OrderStatus = "pending_payment"
	OrderStatusPaid           OrderStatus = "paid"
	OrderStatusPaymentFailed  OrderStatus = "payment_failed"
	OrderStatusRefunded       OrderStatus = "refunded"
	OrderStatusProcessing     OrderStatus = "processing"
	OrderStatusShipped        OrderStatus = "shipped"
	OrderStatusCompleted      OrderStatus = "completed"
	OrderStatusCancelled      OrderStatus = "cancelled"
)

func AllowedOrderStatusTransitions(from OrderStatus) []OrderStatus {
	switch from {
	case OrderStatusPendingPayment:
		return []OrderStatus{OrderStatusPaid, OrderStatusPaymentFailed, OrderStatusCancelled}
	case OrderStatusPaid:
		return []OrderStatus{OrderStatusProcessing, OrderStatusCancelled, OrderStatusRefunded}
	case OrderStatusPending:
		return []OrderStatus{OrderStatusProcessing, OrderStatusCancelled}
	case OrderStatusProcessing:
		// Shipping handover is performed only through ShippingService.Handover.
		// Logistics lifecycle statuses are owned by shipping-service.
		return []OrderStatus{OrderStatusCancelled}
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
	UserID         uint `gorm:"not null;index"`
	User           User
	Status         OrderStatus   `gorm:"size:50;index;not null"`
	OrderNumber    string        `gorm:"size:32;uniqueIndex"`
	IdempotencyKey string        `gorm:"size:64;not null;uniqueIndex:idx_order_user_key"`
	PaymentMethod  PaymentMethod `gorm:"size:32;not null"`
	Currency       string        `gorm:"size:3;not null;default:EUR"`
	SubtotalCents  int64         `gorm:"not null;default:0"`
	ShippingCents  int64         `gorm:"not null;default:0"`
	TaxCents       int64         `gorm:"not null;default:0"`
	DiscountCents  int64         `gorm:"not null;default:0"`
	TotalCents     int64         `gorm:"not null"`
	Items          []OrderItem
	Addresses      []OrderAddress
	Payment        Payment
	Shipment       OrderShipment
}

type OrderShipment struct {
	gorm.Model
	OrderID           uint   `gorm:"not null;uniqueIndex:idx_order_shipments_order_type,priority:1"`
	ShipmentID        string `gorm:"size:64;not null;uniqueIndex"`
	HandoverCode      string `gorm:"size:64;not null"`
	TrackingNumber    string `gorm:"size:64;not null;uniqueIndex"`
	ShipmentType      string `gorm:"size:16;not null;uniqueIndex:idx_order_shipments_order_type,priority:2"`
	Status            string `gorm:"size:50;not null"`
	StatusLabel       string `gorm:"size:100;not null"`
	RemainingStops    *int
	EstimatedFrom     *time.Time
	EstimatedUntil    *time.Time
	LastShippingEvent string `gorm:"size:64"`
}

type ShippingEventReceipt struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	EventID   string `gorm:"size:64;not null;uniqueIndex"`
}

type OrderItem struct {
	gorm.Model
	OrderID        uint    `gorm:"not null;index"`
	ProductID      uint    `gorm:"not null"`
	Product        Product `gorm:"constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	ProductName    string  `gorm:"size:200;not null"`
	ProductSKU     string  `gorm:"size:100;not null"`
	UnitPriceCents int64   `gorm:"not null"`
	Quantity       int     `gorm:"not null"`
	SubtotalCents  int64   `gorm:"not null"`
}

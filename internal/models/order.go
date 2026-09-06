package models

import (
	"time"

	"gorm.io/gorm"
)

type OrderStatus string

const (
	OrderStatusPending          OrderStatus = "pending"
	OrderStatusPendingPayment   OrderStatus = "pending_payment"
	OrderStatusPaid             OrderStatus = "paid"
	OrderStatusPaymentFailed    OrderStatus = "payment_failed"
	OrderStatusRefunded         OrderStatus = "refunded"
	OrderStatusPreparing        OrderStatus = "preparing"
	OrderStatusReadyForShipping OrderStatus = "ready_for_shipping"
	OrderStatusProcessing       OrderStatus = "processing"
	OrderStatusShipped          OrderStatus = "shipped"
	OrderStatusCompleted        OrderStatus = "completed"
	OrderStatusCancelled        OrderStatus = "cancelled"
)

func AllowedOrderStatusTransitions(from OrderStatus) []OrderStatus {
	switch from {
	case OrderStatusPendingPayment:
		return []OrderStatus{OrderStatusPaid, OrderStatusPaymentFailed, OrderStatusCancelled}
	case OrderStatusPaid:
		return []OrderStatus{OrderStatusPreparing, OrderStatusCancelled, OrderStatusRefunded}
	case OrderStatusPending:
		return []OrderStatus{OrderStatusPreparing, OrderStatusCancelled}
	case OrderStatusPreparing, OrderStatusProcessing:
		return []OrderStatus{OrderStatusReadyForShipping, OrderStatusCancelled}
	case OrderStatusReadyForShipping:
		// Shipping handover is performed only through ShippingService.Handover.
		// Logistics lifecycle statuses are owned by shipping-service.
		return []OrderStatus{OrderStatusCancelled}
	case OrderStatusShipped:
		// A shipment can only be cancelled before shipping-service receives it.
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
	UserID             uint `gorm:"not null;index"`
	User               User
	Status             OrderStatus   `gorm:"size:50;index;not null"`
	OrderNumber        string        `gorm:"size:32;uniqueIndex"`
	IdempotencyKey     string        `gorm:"size:64;not null;uniqueIndex:idx_order_user_key"`
	PaymentMethod      PaymentMethod `gorm:"size:32;not null"`
	Currency           string        `gorm:"size:3;not null;default:EUR"`
	SubtotalCents      int64         `gorm:"not null;default:0"`
	ShippingCents      int64         `gorm:"not null;default:0"`
	TaxCents           int64         `gorm:"not null;default:0"`
	DiscountCents      int64         `gorm:"not null;default:0"`
	TotalCents         int64         `gorm:"not null"`
	Items              []OrderItem
	Addresses          []OrderAddress
	Payment            Payment
	Shipment           OrderShipment
	ReturnRequest      *ReturnRequest
	CustomerReceivedAt *time.Time
}

type OrderShipment struct {
	gorm.Model
	OrderID           uint   `gorm:"not null;uniqueIndex:idx_order_shipments_order_type,priority:1"`
	ShipmentID        string `gorm:"size:64;not null;uniqueIndex"`
	HandoverCode      string `gorm:"size:64;not null;index"`
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

type ReturnReason string

const (
	ReturnReasonDamaged        ReturnReason = "damaged"
	ReturnReasonWrongItem      ReturnReason = "wrong_item"
	ReturnReasonNotAsDescribed ReturnReason = "not_as_described"
	ReturnReasonDoesNotFit     ReturnReason = "does_not_fit"
	ReturnReasonChangedMind    ReturnReason = "changed_mind"
	ReturnReasonDefective      ReturnReason = "defective"
	ReturnReasonOther          ReturnReason = "other"
)

func (reason ReturnReason) Valid() bool {
	switch reason {
	case ReturnReasonDamaged, ReturnReasonWrongItem, ReturnReasonNotAsDescribed, ReturnReasonDoesNotFit, ReturnReasonChangedMind, ReturnReasonDefective, ReturnReasonOther:
		return true
	default:
		return false
	}
}

type ReturnRequest struct {
	gorm.Model
	OrderID              uint         `gorm:"not null;uniqueIndex"`
	UserID               uint         `gorm:"not null;index"`
	Reason               ReturnReason `gorm:"size:32;not null"`
	Note                 string       `gorm:"size:1000"`
	OriginalShipmentID   string       `gorm:"size:64;not null;uniqueIndex"`
	ReturnShipmentID     string       `gorm:"size:64;not null;uniqueIndex"`
	ReturnTrackingNumber string       `gorm:"size:64;not null;uniqueIndex"`
	Status               string       `gorm:"size:50;not null"`
	Items                []ReturnItem
}

type ReturnItem struct {
	gorm.Model
	ReturnRequestID uint `gorm:"not null;uniqueIndex:idx_return_items_request_order_item,priority:1"`
	OrderItemID     uint `gorm:"not null;uniqueIndex:idx_return_items_request_order_item,priority:2"`
	ProductID       uint `gorm:"not null"`
	Quantity        int  `gorm:"not null"`
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

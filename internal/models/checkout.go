package models

import (
	"time"

	"gorm.io/gorm"
)

type AddressType string

const (
	AddressTypeShipping AddressType = "shipping"
	AddressTypeBilling  AddressType = "billing"
)

type UserAddress struct {
	gorm.Model
	UserID       uint   `gorm:"not null;index"`
	FirstName    string `gorm:"size:100;not null"`
	LastName     string `gorm:"size:100;not null"`
	Company      string `gorm:"size:150"`
	Street       string `gorm:"size:150;not null"`
	HouseNumber  string `gorm:"size:30;not null"`
	AddressLine2 string `gorm:"column:address_line_2;size:150"`
	PostalCode   string `gorm:"size:20;not null"`
	City         string `gorm:"size:100;not null"`
	State        string `gorm:"size:100"`
	CountryCode  string `gorm:"size:2;not null"`
	Phone        string `gorm:"size:32;not null"`
	IsDefault    bool   `gorm:"not null;default:false"`
}

type OrderAddress struct {
	ID           uint `gorm:"primaryKey"`
	CreatedAt    time.Time
	OrderID      uint        `gorm:"not null;uniqueIndex:idx_order_address_type"`
	Type         AddressType `gorm:"size:16;not null;uniqueIndex:idx_order_address_type"`
	FirstName    string      `gorm:"size:100;not null"`
	LastName     string      `gorm:"size:100;not null"`
	Company      string      `gorm:"size:150"`
	Street       string      `gorm:"size:150;not null"`
	HouseNumber  string      `gorm:"size:30;not null"`
	AddressLine2 string      `gorm:"column:address_line_2;size:150"`
	PostalCode   string      `gorm:"size:20;not null"`
	City         string      `gorm:"size:100;not null"`
	State        string      `gorm:"size:100"`
	CountryCode  string      `gorm:"size:2;not null"`
	Phone        string      `gorm:"size:32;not null"`
}

type PaymentMethod string

const (
	PaymentMethodDebitCard  PaymentMethod = "debit_card"
	PaymentMethodCreditCard PaymentMethod = "credit_card"
	PaymentMethodPayPal     PaymentMethod = "paypal"
	PaymentMethodKlarna     PaymentMethod = "klarna"
)

func (method PaymentMethod) Valid() bool {
	switch method {
	case PaymentMethodDebitCard, PaymentMethodCreditCard, PaymentMethodPayPal, PaymentMethodKlarna:
		return true
	default:
		return false
	}
}

func (method PaymentMethod) Provider() string {
	switch method {
	case PaymentMethodDebitCard, PaymentMethodCreditCard:
		return "card"
	case PaymentMethodPayPal:
		return "paypal"
	case PaymentMethodKlarna:
		return "klarna"
	default:
		return ""
	}
}

type PaymentStatus string

const (
	PaymentStatusPending           PaymentStatus = "pending"
	PaymentStatusAuthorized        PaymentStatus = "authorized"
	PaymentStatusPaid              PaymentStatus = "paid"
	PaymentStatusFailed            PaymentStatus = "failed"
	PaymentStatusCancelled         PaymentStatus = "cancelled"
	PaymentStatusRefunded          PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded PaymentStatus = "partially_refunded"
)

type Payment struct {
	gorm.Model
	OrderID           uint          `gorm:"not null;uniqueIndex"`
	UserID            uint          `gorm:"not null;uniqueIndex:idx_payment_user_key;index"`
	Provider          string        `gorm:"size:32;not null;uniqueIndex:idx_payment_provider_id"`
	Method            PaymentMethod `gorm:"size:32;not null"`
	ProviderPaymentID string        `gorm:"size:128;not null;uniqueIndex:idx_payment_provider_id"`
	Status            PaymentStatus `gorm:"size:32;not null;index"`
	AmountCents       int64         `gorm:"not null"`
	Currency          string        `gorm:"size:3;not null"`
	IdempotencyKey    string        `gorm:"size:64;not null;uniqueIndex:idx_payment_user_key"`
	Brand             string        `gorm:"size:32"`
	Last4             string        `gorm:"size:4"`
}

type CheckoutAttempt struct {
	ID             uint `gorm:"primaryKey"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	UserID         uint   `gorm:"not null;uniqueIndex:idx_checkout_user_key"`
	IdempotencyKey string `gorm:"size:64;not null;uniqueIndex:idx_checkout_user_key"`
	RequestHash    string `gorm:"size:64;not null"`
	Status         string `gorm:"size:20;not null"`
	OrderID        *uint  `gorm:"uniqueIndex"`
}

type WebhookEvent struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	Provider  string `gorm:"size:32;not null;uniqueIndex:idx_webhook_provider_event"`
	EventID   string `gorm:"size:128;not null;uniqueIndex:idx_webhook_provider_event"`
}

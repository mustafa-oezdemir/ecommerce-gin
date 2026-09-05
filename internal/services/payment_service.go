package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidPayment = errors.New("invalid payment")
	ErrPaymentFailed  = errors.New("payment failed")
	ErrInvalidWebhook = errors.New("invalid webhook")
)

type PaymentRequest struct {
	UserID         uint
	Method         models.PaymentMethod
	AmountCents    int64
	Currency       string
	IdempotencyKey string
	ProviderToken  string
}

type PaymentResult struct {
	ProviderPaymentID string
	Status            models.PaymentStatus
	Brand             string
	Last4             string
}

type PaymentGateway interface {
	CreatePayment(context.Context, PaymentRequest) (*PaymentResult, error)
	ConfirmPayment(context.Context, string) (*PaymentResult, error)
	GetPaymentStatus(context.Context, string) (models.PaymentStatus, error)
	RefundPayment(context.Context, string, int64) error
}

// ReferencePaymentGateway models provider tokenization/idempotency without ever
// accepting raw card data. Production deployments can replace this adapter.
type ReferencePaymentGateway struct{ enabled bool }

func NewReferencePaymentGateway(environment string) *ReferencePaymentGateway {
	return &ReferencePaymentGateway{enabled: environment != "production"}
}

func (gateway *ReferencePaymentGateway) CreatePayment(_ context.Context, request PaymentRequest) (*PaymentResult, error) {
	if !gateway.enabled {
		return nil, ErrPaymentFailed
	}
	if request.UserID == 0 || !request.Method.Valid() || request.AmountCents <= 0 || request.Currency != "EUR" || !validIdempotencyKey(request.IdempotencyKey) {
		return nil, ErrInvalidPayment
	}
	// A real adapter passes the same key to the provider. This deterministic ID
	// gives the local reference adapter identical retry semantics without state.
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", request.Method.Provider(), request.UserID, request.IdempotencyKey)))
	return &PaymentResult{ProviderPaymentID: "ref_" + hex.EncodeToString(digest[:16]), Status: models.PaymentStatusPaid}, nil
}

func (gateway *ReferencePaymentGateway) ConfirmPayment(_ context.Context, paymentID string) (*PaymentResult, error) {
	if !gateway.enabled || !strings.HasPrefix(paymentID, "ref_") {
		return nil, ErrInvalidPayment
	}
	return &PaymentResult{ProviderPaymentID: paymentID, Status: models.PaymentStatusPaid}, nil
}
func (gateway *ReferencePaymentGateway) GetPaymentStatus(_ context.Context, paymentID string) (models.PaymentStatus, error) {
	if !gateway.enabled || !strings.HasPrefix(paymentID, "ref_") {
		return "", ErrInvalidPayment
	}
	return models.PaymentStatusPaid, nil
}
func (gateway *ReferencePaymentGateway) RefundPayment(_ context.Context, paymentID string, amount int64) error {
	if !gateway.enabled || !strings.HasPrefix(paymentID, "ref_") || amount <= 0 {
		return ErrInvalidPayment
	}
	return nil
}

type PaymentWebhook struct {
	EventID           string               `json:"event_id"`
	ProviderPaymentID string               `json:"provider_payment_id"`
	Status            models.PaymentStatus `json:"status"`
}

type PaymentWebhookService struct {
	database *gorm.DB
	secret   []byte
}

func NewPaymentWebhookService(database *gorm.DB, secret string) *PaymentWebhookService {
	return &PaymentWebhookService{database: database, secret: []byte(strings.TrimSpace(secret))}
}

func (s *PaymentWebhookService) VerifySignature(body []byte, signature string) bool {
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(s.secret) < 32 {
		return false
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (s *PaymentWebhookService) Process(ctx context.Context, provider string, event PaymentWebhook) (bool, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" || event.EventID == "" || event.ProviderPaymentID == "" {
		return false, ErrInvalidWebhook
	}
	if event.Status != models.PaymentStatusPaid && event.Status != models.PaymentStatusFailed && event.Status != models.PaymentStatusRefunded {
		return false, ErrInvalidWebhook
	}
	replayed := false
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record := models.WebhookEvent{Provider: provider, EventID: event.EventID}
		if err := tx.Create(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				replayed = true
				return nil
			}
			return err
		}
		var payment models.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND provider_payment_id = ?", provider, event.ProviderPaymentID).First(&payment).Error; err != nil {
			return ErrInvalidWebhook
		}
		if !canTransitionPaymentStatus(payment.Status, event.Status) {
			return ErrInvalidWebhook
		}
		releaseStock := payment.Status != models.PaymentStatusFailed && event.Status == models.PaymentStatusFailed
		if err := tx.Model(&payment).Update("status", event.Status).Error; err != nil {
			return err
		}
		orderStatus := models.OrderStatusPaid
		switch event.Status {
		case models.PaymentStatusFailed:
			orderStatus = models.OrderStatusPaymentFailed
		case models.PaymentStatusRefunded:
			orderStatus = models.OrderStatusRefunded
		}
		if err := tx.Model(&models.Order{}).Where("id = ?", payment.OrderID).Update("status", orderStatus).Error; err != nil {
			return fmt.Errorf("update webhook order: %w", err)
		}
		if releaseStock {
			var items []models.OrderItem
			if err := tx.Where("order_id = ?", payment.OrderID).Order("product_id ASC").Find(&items).Error; err != nil {
				return fmt.Errorf("load failed payment items: %w", err)
			}
			for _, item := range items {
				if err := tx.Model(&models.Product{}).Where("id = ?", item.ProductID).Update("stock", gorm.Expr("stock + ?", item.Quantity)).Error; err != nil {
					return fmt.Errorf("release failed payment stock: %w", err)
				}
			}
		}
		return nil
	})
	return replayed, err
}

func canTransitionPaymentStatus(from, to models.PaymentStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case models.PaymentStatusPending:
		return to == models.PaymentStatusAuthorized || to == models.PaymentStatusPaid || to == models.PaymentStatusFailed || to == models.PaymentStatusCancelled
	case models.PaymentStatusAuthorized:
		return to == models.PaymentStatusPaid || to == models.PaymentStatusFailed || to == models.PaymentStatusCancelled
	case models.PaymentStatusPaid:
		return to == models.PaymentStatusRefunded || to == models.PaymentStatusPartiallyRefunded
	case models.PaymentStatusPartiallyRefunded:
		return to == models.PaymentStatusRefunded
	default:
		return false
	}
}

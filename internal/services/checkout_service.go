package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const checkoutShippingCents int64 = 499

var (
	ErrInvalidCheckout     = errors.New("invalid checkout")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for another request")
	errIdempotencyReplay   = errors.New("idempotency replay")
)

type CheckoutInput struct {
	UserID                uint
	ShippingAddressID     uint
	BillingAddressID      uint
	BillingSameAsShipping bool
	PaymentMethod         models.PaymentMethod
	IdempotencyKey        string
	ProviderToken         string
}

type CheckoutSummary struct {
	Cart          *models.Cart
	SubtotalCents int64
	ShippingCents int64
	TaxCents      int64
	DiscountCents int64
	TotalCents    int64
}

type CheckoutResult struct {
	Order  *models.Order
	Replay bool
}

type CheckoutService struct {
	database *gorm.DB
	gateway  PaymentGateway
}

func NewCheckoutService(database *gorm.DB, gateway PaymentGateway) *CheckoutService {
	if database == nil || gateway == nil {
		panic("services: checkout dependencies are required")
	}
	return &CheckoutService{database: database, gateway: gateway}
}

func NewIdempotencyKey() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate idempotency key: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func validIdempotencyKey(value string) bool {
	if len(value) < 32 || len(value) > 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *CheckoutService) Preview(ctx context.Context, userID uint) (*CheckoutSummary, error) {
	if userID == 0 {
		return nil, ErrInvalidUser
	}
	var cart models.Cart
	if err := s.database.WithContext(ctx).Preload("Items.Product").Where("user_id = ?", userID).First(&cart).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCartNotFound
		}
		return nil, err
	}
	if len(cart.Items) == 0 {
		return nil, ErrCartEmpty
	}
	summary := &CheckoutSummary{Cart: &cart, ShippingCents: checkoutShippingCents}
	for _, item := range cart.Items {
		if item.Quantity < 1 || item.Quantity > 100 || !item.Product.Active || item.Product.PriceCents <= 0 {
			return nil, ErrProductUnavailable
		}
		summary.SubtotalCents += item.Product.PriceCents * int64(item.Quantity)
	}
	summary.TotalCents = summary.SubtotalCents + summary.ShippingCents
	return summary, nil
}

func (s *CheckoutService) Checkout(ctx context.Context, input CheckoutInput) (*CheckoutResult, error) {
	if input.UserID == 0 || input.ShippingAddressID == 0 || !input.PaymentMethod.Valid() || !validIdempotencyKey(input.IdempotencyKey) {
		return nil, ErrInvalidCheckout
	}
	if input.BillingSameAsShipping {
		input.BillingAddressID = input.ShippingAddressID
	}
	if input.BillingAddressID == 0 {
		return nil, ErrInvalidCheckout
	}
	requestHash := checkoutRequestHash(input)
	var created models.Order
	var paymentResult *PaymentResult
	var chargedAmount int64
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt := models.CheckoutAttempt{UserID: input.UserID, IdempotencyKey: input.IdempotencyKey, RequestHash: requestHash, Status: "processing"}
		if err := tx.Create(&attempt).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return errIdempotencyReplay
			}
			return fmt.Errorf("create checkout attempt: %w", err)
		}

		shipping, err := loadOwnedAddress(tx, input.UserID, input.ShippingAddressID)
		if err != nil {
			return err
		}
		billing := shipping
		if input.BillingAddressID != input.ShippingAddressID {
			billing, err = loadOwnedAddress(tx, input.UserID, input.BillingAddressID)
			if err != nil {
				return err
			}
		}

		var cart models.Cart
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", input.UserID).First(&cart).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCartNotFound
			}
			return fmt.Errorf("load checkout cart: %w", err)
		}
		var cartItems []models.CartItem
		if err := tx.Where("cart_id = ?", cart.ID).Find(&cartItems).Error; err != nil {
			return fmt.Errorf("load checkout items: %w", err)
		}
		if len(cartItems) == 0 {
			return ErrCartEmpty
		}
		sort.Slice(cartItems, func(i, j int) bool { return cartItems[i].ProductID < cartItems[j].ProductID })

		items := make([]models.OrderItem, 0, len(cartItems))
		var subtotal int64
		for _, cartItem := range cartItems {
			if cartItem.Quantity < 1 || cartItem.Quantity > 100 {
				return ErrInvalidQuantity
			}
			var product models.Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND active = ?", cartItem.ProductID, true).First(&product).Error; err != nil {
				return ErrProductUnavailable
			}
			if product.PriceCents <= 0 || product.Stock < cartItem.Quantity {
				return ErrInsufficientStock
			}
			lineTotal := product.PriceCents * int64(cartItem.Quantity)
			if lineTotal <= 0 || subtotal > (1<<63-1)-lineTotal {
				return ErrInvalidCheckout
			}
			subtotal += lineTotal
			items = append(items, models.OrderItem{ProductID: product.ID, ProductName: product.Name, ProductSKU: fmt.Sprintf("SKU-%08d", product.ID), UnitPriceCents: product.PriceCents, Quantity: cartItem.Quantity, SubtotalCents: lineTotal})
		}
		chargedAmount = subtotal + checkoutShippingCents
		paymentResult, err = s.gateway.CreatePayment(ctx, PaymentRequest{UserID: input.UserID, Method: input.PaymentMethod, AmountCents: chargedAmount, Currency: "EUR", IdempotencyKey: input.IdempotencyKey, ProviderToken: input.ProviderToken})
		if err != nil || paymentResult == nil || (paymentResult.Status != models.PaymentStatusPaid && paymentResult.Status != models.PaymentStatusAuthorized) {
			return ErrPaymentFailed
		}
		orderStatus := models.OrderStatusPaid
		if paymentResult.Status == models.PaymentStatusAuthorized {
			orderStatus = models.OrderStatusPendingPayment
		}
		order := models.Order{UserID: input.UserID, Status: orderStatus, IdempotencyKey: input.IdempotencyKey, PaymentMethod: input.PaymentMethod, Currency: "EUR", SubtotalCents: subtotal, ShippingCents: checkoutShippingCents, TotalCents: chargedAmount}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("create checkout order: %w", err)
		}
		order.OrderNumber = fmt.Sprintf("ORD-%010d", order.ID)
		if err := tx.Model(&order).Update("order_number", order.OrderNumber).Error; err != nil {
			return err
		}
		for index := range items {
			items[index].OrderID = order.ID
			result := tx.Model(&models.Product{}).Where("id = ? AND stock >= ?", items[index].ProductID, items[index].Quantity).Update("stock", gorm.Expr("stock - ?", items[index].Quantity))
			if result.Error != nil {
				return fmt.Errorf("decrement checkout stock: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrInsufficientStock
			}
		}
		if err := tx.Create(&items).Error; err != nil {
			return fmt.Errorf("create checkout items: %w", err)
		}
		addresses := []models.OrderAddress{snapshotAddress(order.ID, models.AddressTypeShipping, shipping), snapshotAddress(order.ID, models.AddressTypeBilling, billing)}
		if err := tx.Create(&addresses).Error; err != nil {
			return fmt.Errorf("snapshot checkout addresses: %w", err)
		}
		payment := models.Payment{OrderID: order.ID, UserID: input.UserID, Provider: input.PaymentMethod.Provider(), Method: input.PaymentMethod, ProviderPaymentID: paymentResult.ProviderPaymentID, Status: paymentResult.Status, AmountCents: chargedAmount, Currency: "EUR", IdempotencyKey: input.IdempotencyKey, Brand: paymentResult.Brand, Last4: paymentResult.Last4}
		if err := tx.Create(&payment).Error; err != nil {
			return fmt.Errorf("create payment: %w", err)
		}
		if err := tx.Where("cart_id = ?", cart.ID).Delete(&models.CartItem{}).Error; err != nil {
			return fmt.Errorf("clear checkout cart: %w", err)
		}
		if err := tx.Model(&attempt).Updates(map[string]any{"status": "completed", "order_id": order.ID}).Error; err != nil {
			return err
		}
		order.Items, order.Addresses, order.Payment = items, addresses, payment
		created = order
		return nil
	})
	if errors.Is(err, errIdempotencyReplay) {
		order, replayErr := s.loadReplay(ctx, input.UserID, input.IdempotencyKey, requestHash)
		if replayErr != nil {
			return nil, replayErr
		}
		recordCheckoutMetric("replay")
		slog.InfoContext(ctx, "checkout idempotency replay", "event", "idempotency_replay", "user_id", input.UserID)
		return &CheckoutResult{Order: order, Replay: true}, nil
	}
	if err != nil {
		if paymentResult != nil && paymentResult.Status == models.PaymentStatusPaid {
			if refundErr := s.gateway.RefundPayment(ctx, paymentResult.ProviderPaymentID, chargedAmount); refundErr != nil {
				slog.ErrorContext(ctx, "checkout compensation failed", "event", "payment_refund_failed", "provider", input.PaymentMethod.Provider())
			}
		}
		return nil, err
	}
	recordCheckoutMetric("completed")
	if metric := metrics.Default(); metric != nil {
		metric.OrdersCreated.WithLabelValues(string(created.Status)).Inc()
		metric.OrderValueCents.Observe(float64(created.TotalCents))
	}
	slog.InfoContext(ctx, "checkout completed", "event", "order_created", "order_id", created.ID, "payment_provider", input.PaymentMethod.Provider())
	return &CheckoutResult{Order: &created}, nil
}

func (s *CheckoutService) loadReplay(ctx context.Context, userID uint, key, requestHash string) (*models.Order, error) {
	var attempt models.CheckoutAttempt
	if err := s.database.WithContext(ctx).Where("user_id = ? AND idempotency_key = ?", userID, key).First(&attempt).Error; err != nil {
		return nil, err
	}
	if !hmacEqualString(attempt.RequestHash, requestHash) {
		return nil, ErrIdempotencyConflict
	}
	if attempt.OrderID == nil {
		return nil, ErrInvalidCheckout
	}
	var order models.Order
	if err := s.database.WithContext(ctx).Preload("Items").Preload("Addresses").Preload("Payment").Where("id = ? AND user_id = ?", *attempt.OrderID, userID).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func checkoutRequestHash(input CheckoutInput) string {
	value := fmt.Sprintf("%d:%d:%t:%s", input.ShippingAddressID, input.BillingAddressID, input.BillingSameAsShipping, input.PaymentMethod)
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func hmacEqualString(left, right string) bool {
	leftBytes, leftErr := hex.DecodeString(left)
	rightBytes, rightErr := hex.DecodeString(right)
	return leftErr == nil && rightErr == nil && len(leftBytes) == len(rightBytes) && subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}

func loadOwnedAddress(tx *gorm.DB, userID, addressID uint) (*models.UserAddress, error) {
	var address models.UserAddress
	if err := tx.Where("id = ? AND user_id = ?", addressID, userID).First(&address).Error; err != nil {
		return nil, ErrAddressNotFound
	}
	return &address, nil
}

func snapshotAddress(orderID uint, addressType models.AddressType, address *models.UserAddress) models.OrderAddress {
	return models.OrderAddress{OrderID: orderID, Type: addressType, FirstName: address.FirstName, LastName: address.LastName, Company: address.Company, Street: address.Street, HouseNumber: address.HouseNumber, AddressLine2: address.AddressLine2, PostalCode: address.PostalCode, City: address.City, State: address.State, CountryCode: address.CountryCode, Phone: address.Phone}
}

func recordCheckoutMetric(kind string) {
	metric := metrics.Default()
	if metric == nil {
		return
	}
	switch kind {
	case "completed":
		metric.CheckoutCompleted.Inc()
	case "replay":
		metric.IdempotencyReplays.Inc()
	}
}

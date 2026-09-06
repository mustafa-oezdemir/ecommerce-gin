package services

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func checkoutIntegrationDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("CHECKOUT_TEST_DSN")
	if dsn == "" {
		t.Skip("CHECKOUT_TEST_DSN is not configured")
	}
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open checkout integration database: %v", err)
	}
	return database
}

type checkoutFixture struct {
	users     []models.User
	addresses []models.UserAddress
	carts     []models.Cart
	products  []models.Product
}

func newCheckoutFixture(t *testing.T, database *gorm.DB, userCount, productCount int) checkoutFixture {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fixture := checkoutFixture{}
	for index := 0; index < userCount; index++ {
		user := models.User{Name: fmt.Sprintf("Checkout User %d", index), FirstName: "Checkout", LastName: "User", Email: fmt.Sprintf("checkout-%s-%d@example.test", suffix, index), Password: "not-used", Role: models.RoleCustomer, SecurityVersion: 1}
		if err := database.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		address := models.UserAddress{UserID: user.ID, FirstName: "Checkout", LastName: "User", Street: "Main Street", HouseNumber: "1", PostalCode: "35037", City: "Marburg", CountryCode: "DE", Phone: "+491234567"}
		if err := database.Create(&address).Error; err != nil {
			t.Fatal(err)
		}
		cart := models.Cart{UserID: user.ID}
		if err := database.Create(&cart).Error; err != nil {
			t.Fatal(err)
		}
		fixture.users = append(fixture.users, user)
		fixture.addresses = append(fixture.addresses, address)
		fixture.carts = append(fixture.carts, cart)
	}
	for index := 0; index < productCount; index++ {
		product := models.Product{Name: fmt.Sprintf("Checkout Product %s %d", suffix, index), PriceCents: 1000 + int64(index), Stock: 1, Active: true}
		if err := database.Create(&product).Error; err != nil {
			t.Fatal(err)
		}
		fixture.products = append(fixture.products, product)
	}
	t.Cleanup(func() { cleanupCheckoutFixture(database, fixture) })
	return fixture
}

func cleanupCheckoutFixture(database *gorm.DB, fixture checkoutFixture) {
	userIDs := make([]uint, len(fixture.users))
	productIDs := make([]uint, len(fixture.products))
	for index := range fixture.users {
		userIDs[index] = fixture.users[index].ID
	}
	for index := range fixture.products {
		productIDs[index] = fixture.products[index].ID
	}
	var orderIDs []uint
	database.Unscoped().Model(&models.Order{}).Where("user_id IN ?", userIDs).Pluck("id", &orderIDs)
	eventIDs := make([]string, len(orderIDs))
	for index, orderID := range orderIDs {
		eventIDs[index] = "event-" + fmt.Sprint(orderID)
	}
	database.Where("event_id IN ?", eventIDs).Delete(&models.WebhookEvent{})
	database.Unscoped().Where("return_request_id IN (SELECT id FROM return_requests WHERE order_id IN ?)", orderIDs).Delete(&models.ReturnItem{})
	database.Unscoped().Where("order_id IN ?", orderIDs).Delete(&models.ReturnRequest{})
	database.Unscoped().Where("order_id IN ?", orderIDs).Delete(&models.OrderShipment{})
	database.Unscoped().Where("order_id IN ?", orderIDs).Delete(&models.Payment{})
	database.Unscoped().Where("order_id IN ?", orderIDs).Delete(&models.OrderAddress{})
	database.Unscoped().Where("order_id IN ?", orderIDs).Delete(&models.OrderItem{})
	database.Unscoped().Where("user_id IN ?", userIDs).Delete(&models.CheckoutAttempt{})
	database.Unscoped().Where("id IN ?", orderIDs).Delete(&models.Order{})
	database.Unscoped().Where("cart_id IN ?", cartIDs(fixture.carts)).Delete(&models.CartItem{})
	database.Unscoped().Where("id IN ?", cartIDs(fixture.carts)).Delete(&models.Cart{})
	database.Unscoped().Where("user_id IN ?", userIDs).Delete(&models.UserAddress{})
	database.Unscoped().Where("id IN ?", userIDs).Delete(&models.User{})
	database.Unscoped().Where("id IN ?", productIDs).Delete(&models.Product{})
}

func cartIDs(carts []models.Cart) []uint {
	ids := make([]uint, len(carts))
	for index := range carts {
		ids[index] = carts[index].ID
	}
	return ids
}

func TestConcurrentCheckoutCannotOversellLastProduct(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 2, 1)
	for index := range fixture.carts {
		if err := database.Create(&models.CartItem{CartID: fixture.carts[index].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	var successes atomic.Int32
	var stockFailures atomic.Int32
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			key := fmt.Sprintf("%064x", index+1)
			_, err := service.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[index].ID, ShippingAddressID: fixture.addresses[index].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodCreditCard, IdempotencyKey: key})
			if err == nil {
				successes.Add(1)
			} else if errors.Is(err, ErrInsufficientStock) || errors.Is(err, ErrProductUnavailable) {
				stockFailures.Add(1)
			} else {
				t.Errorf("unexpected checkout error: %v", err)
			}
		}(index)
	}
	close(start)
	wait.Wait()
	var product models.Product
	database.First(&product, fixture.products[0].ID)
	var orderCount int64
	database.Model(&models.Order{}).Where("user_id IN ?", []uint{fixture.users[0].ID, fixture.users[1].ID}).Count(&orderCount)
	if successes.Load() != 1 || stockFailures.Load() != 1 || product.Stock != 0 || orderCount != 1 {
		t.Fatalf("success=%d stock failures=%d final stock=%d orders=%d", successes.Load(), stockFailures.Load(), product.Stock, orderCount)
	}
}

func TestConcurrentSameIdempotencyKeyCreatesOneOrderAndPayment(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	if err := database.Model(&fixture.products[0]).Update("stock", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.CartItem{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	input := CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodPayPal, IdempotencyKey: fmt.Sprintf("%064x", 44)}
	start := make(chan struct{})
	results := make(chan *CheckoutResult, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := service.Checkout(t.Context(), input)
			results <- result
			errorsChannel <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsChannel)
	var orderID uint
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("idempotent checkout error: %v", err)
		}
	}
	for result := range results {
		if result == nil || result.Order == nil {
			t.Fatal("missing checkout result")
		}
		if orderID != 0 && orderID != result.Order.ID {
			t.Fatalf("different orders returned: %d and %d", orderID, result.Order.ID)
		}
		orderID = result.Order.ID
	}
	var orderCount, paymentCount int64
	database.Model(&models.Order{}).Where("user_id = ?", fixture.users[0].ID).Count(&orderCount)
	database.Model(&models.Payment{}).Where("user_id = ?", fixture.users[0].ID).Count(&paymentCount)
	var product models.Product
	database.First(&product, fixture.products[0].ID)
	if orderCount != 1 || paymentCount != 1 || product.Stock != 1 {
		t.Fatalf("orders=%d payments=%d stock=%d", orderCount, paymentCount, product.Stock)
	}
	conflicting := input
	conflicting.PaymentMethod = models.PaymentMethodKlarna
	if _, err := service.Checkout(t.Context(), conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key with changed request error = %v", err)
	}
}

func TestMultiItemCheckoutRollsBackAllStockAndKeepsCart(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 2)
	if err := database.Model(&fixture.products[0]).Update("stock", 10).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&fixture.products[1]).Update("stock", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&[]models.CartItem{{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}, {CartID: fixture.carts[0].ID, ProductID: fixture.products[1].ID, Quantity: 1}}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	_, err := service.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodKlarna, IdempotencyKey: fmt.Sprintf("%064x", 55)})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("checkout error = %v", err)
	}
	var first, second models.Product
	database.First(&first, fixture.products[0].ID)
	database.First(&second, fixture.products[1].ID)
	var orderCount, cartItemCount int64
	database.Model(&models.Order{}).Where("user_id = ?", fixture.users[0].ID).Count(&orderCount)
	database.Model(&models.CartItem{}).Where("cart_id = ?", fixture.carts[0].ID).Count(&cartItemCount)
	if first.Stock != 10 || second.Stock != 0 || orderCount != 0 || cartItemCount != 2 {
		t.Fatalf("stock=(%d,%d) orders=%d cart items=%d", first.Stock, second.Stock, orderCount, cartItemCount)
	}
}

func TestPaymentRetryCreatesOneLogicalPaymentAndKeepsSnapshot(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	if err := database.Create(&models.CartItem{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &retryGateway{}
	service := NewCheckoutService(database, gateway)
	input := CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodDebitCard, IdempotencyKey: fmt.Sprintf("%064x", 66)}
	if _, err := service.Checkout(t.Context(), input); !errors.Is(err, ErrPaymentFailed) {
		t.Fatalf("first payment error = %v", err)
	}
	result, err := service.Checkout(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.UserAddress{}).Where("id = ?", fixture.addresses[0].ID).Update("street", "Changed Street").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.Product{}).Where("id = ?", fixture.products[0].ID).Updates(map[string]any{"name": "Changed Product", "price_cents": 9999}).Error; err != nil {
		t.Fatal(err)
	}
	var paymentCount int64
	database.Model(&models.Payment{}).Where("user_id = ?", fixture.users[0].ID).Count(&paymentCount)
	var snapshot models.OrderAddress
	database.Where("order_id = ? AND type = ?", result.Order.ID, models.AddressTypeShipping).First(&snapshot)
	var itemSnapshot models.OrderItem
	database.Where("order_id = ?", result.Order.ID).First(&itemSnapshot)
	var cartItemCount int64
	database.Model(&models.CartItem{}).Where("cart_id = ?", fixture.carts[0].ID).Count(&cartItemCount)
	if paymentCount != 1 || len(gateway.logicalIDs) != 1 || snapshot.Street != "Main Street" || itemSnapshot.ProductName == "Changed Product" || itemSnapshot.UnitPriceCents != 1000 || cartItemCount != 0 {
		t.Fatalf("payments=%d logical=%d address=%q item=%q/%d cart=%d", paymentCount, len(gateway.logicalIDs), snapshot.Street, itemSnapshot.ProductName, itemSnapshot.UnitPriceCents, cartItemCount)
	}
}

func TestWebhookProcessingIsIdempotent(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	if err := database.Create(&models.CartItem{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	result, err := service.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodCreditCard, IdempotencyKey: fmt.Sprintf("%064x", 77)})
	if err != nil {
		t.Fatal(err)
	}
	webhooks := NewPaymentWebhookService(database, "0123456789abcdef0123456789abcdef")
	event := PaymentWebhook{EventID: "event-" + fmt.Sprint(result.Order.ID), ProviderPaymentID: result.Order.Payment.ProviderPaymentID, Status: models.PaymentStatusPaid}
	replay, err := webhooks.Process(t.Context(), "card", event)
	if err != nil || replay {
		t.Fatalf("first webhook replay=%v error=%v", replay, err)
	}
	replay, err = webhooks.Process(t.Context(), "card", event)
	if err != nil || !replay {
		t.Fatalf("second webhook replay=%v error=%v", replay, err)
	}
	var eventCount int64
	database.Model(&models.WebhookEvent{}).Where("provider = ? AND event_id = ?", "card", event.EventID).Count(&eventCount)
	if eventCount != 1 {
		t.Fatalf("webhook event count = %d", eventCount)
	}
}

func TestSameIdempotencyKeyIsIsolatedByUser(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 2, 2)
	for index := range fixture.users {
		if err := database.Create(&models.CartItem{CartID: fixture.carts[index].ID, ProductID: fixture.products[index].ID, Quantity: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	key := fmt.Sprintf("%064x", 88)
	for index := range fixture.users {
		if _, err := service.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[index].ID, ShippingAddressID: fixture.addresses[index].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodPayPal, IdempotencyKey: key}); err != nil {
			t.Fatalf("user %d checkout: %v", index, err)
		}
	}
	var orderCount int64
	database.Model(&models.Order{}).Where("user_id IN ?", []uint{fixture.users[0].ID, fixture.users[1].ID}).Count(&orderCount)
	if orderCount != 2 {
		t.Fatalf("same key across users created %d orders, want 2", orderCount)
	}
}

func TestDifferentIdempotencyKeyAllowsNewCheckout(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	if err := database.Model(&fixture.products[0]).Update("stock", 2).Error; err != nil {
		t.Fatal(err)
	}
	service := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	for index := 1; index <= 2; index++ {
		if index == 1 {
			if err := database.Create(&models.CartItem{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := database.Unscoped().Model(&models.CartItem{}).Where("cart_id = ? AND product_id = ?", fixture.carts[0].ID, fixture.products[0].ID).Updates(map[string]any{"deleted_at": nil, "quantity": 1}).Error; err != nil {
				t.Fatal(err)
			}
		}
		_, err := service.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodDebitCard, IdempotencyKey: fmt.Sprintf("%064x", 200+index)})
		if err != nil {
			t.Fatalf("checkout %d: %v", index, err)
		}
	}
	var orderCount int64
	database.Model(&models.Order{}).Where("user_id = ?", fixture.users[0].ID).Count(&orderCount)
	if orderCount != 2 {
		t.Fatalf("different keys created %d orders, want 2", orderCount)
	}
}

func TestAddressIsolationCRUDAndSeparateBillingSnapshot(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 2, 1)
	addresses := NewAddressService(database)
	replacement := models.UserAddress{FirstName: "Other", LastName: "Person", Street: "Other Street", HouseNumber: "2", PostalCode: "10115", City: "Berlin", CountryCode: "DE", Phone: "+49000"}
	if err := addresses.Update(t.Context(), fixture.users[0].ID, fixture.addresses[1].ID, replacement); !errors.Is(err, ErrAddressNotFound) {
		t.Fatalf("cross-user address update error = %v", err)
	}
	if err := addresses.Delete(t.Context(), fixture.users[0].ID, fixture.addresses[1].ID); !errors.Is(err, ErrAddressNotFound) {
		t.Fatalf("cross-user address delete error = %v", err)
	}
	billing, err := addresses.Create(t.Context(), fixture.users[0].ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	replacement.City = "Hamburg"
	if err := addresses.Update(t.Context(), fixture.users[0].ID, billing.ID, replacement); err != nil {
		t.Fatalf("update own address: %v", err)
	}
	if err := database.Create(&models.CartItem{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
		t.Fatal(err)
	}
	checkout := NewCheckoutService(database, NewReferencePaymentGateway("test"))
	result, err := checkout.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingAddressID: billing.ID, PaymentMethod: models.PaymentMethodCreditCard, IdempotencyKey: fmt.Sprintf("%064x", 99)})
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []models.OrderAddress
	if err := database.Where("order_id = ?", result.Order.ID).Order("type").Find(&snapshots).Error; err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 || snapshots[0].Street == snapshots[1].Street {
		t.Fatalf("separate billing snapshot was not retained: %#v", snapshots)
	}
	if err := addresses.Delete(t.Context(), fixture.users[0].ID, billing.ID); err != nil {
		t.Fatal(err)
	}
}

func TestFailedAuthorizedPaymentReleasesStockOnlyOnce(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	if err := database.Create(&models.CartItem{CartID: fixture.carts[0].ID, ProductID: fixture.products[0].ID, Quantity: 1}).Error; err != nil {
		t.Fatal(err)
	}
	checkout := NewCheckoutService(database, authorizedGateway{})
	result, err := checkout.Checkout(t.Context(), CheckoutInput{UserID: fixture.users[0].ID, ShippingAddressID: fixture.addresses[0].ID, BillingSameAsShipping: true, PaymentMethod: models.PaymentMethodKlarna, IdempotencyKey: fmt.Sprintf("%064x", 111)})
	if err != nil {
		t.Fatal(err)
	}
	webhooks := NewPaymentWebhookService(database, "0123456789abcdef0123456789abcdef")
	for index, eventID := range []string{"event-failed-" + fmt.Sprint(result.Order.ID), "event-failed-again-" + fmt.Sprint(result.Order.ID)} {
		replay, err := webhooks.Process(t.Context(), "klarna", PaymentWebhook{EventID: eventID, ProviderPaymentID: result.Order.Payment.ProviderPaymentID, Status: models.PaymentStatusFailed})
		if err != nil || replay {
			t.Fatalf("failed webhook %d replay=%v error=%v", index, replay, err)
		}
	}
	var product models.Product
	database.First(&product, fixture.products[0].ID)
	if product.Stock != 1 {
		t.Fatalf("failed payment restored stock to %d, want 1", product.Stock)
	}
}

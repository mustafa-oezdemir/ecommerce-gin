package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	shippingapi "github.com/mustafa-oezdemir/ecommerce-gin/internal/shipping"
	"gorm.io/gorm"
)

type shippingClientStub struct {
	createdRequest shippingapi.CreateShipmentRequest
	returnRequest  shippingapi.CreateReturnRequest
	idempotencyKey string
	requestID      string
	shipment       *shippingapi.Shipment
	returnShipment *shippingapi.Shipment
	err            error
}

func (stub *shippingClientStub) CreateShipment(_ context.Context, request shippingapi.CreateShipmentRequest, key, requestID string) (*shippingapi.Shipment, error) {
	stub.createdRequest, stub.idempotencyKey, stub.requestID = request, key, requestID
	return stub.shipment, stub.err
}
func (stub *shippingClientStub) CreateReturn(_ context.Context, request shippingapi.CreateReturnRequest, key, requestID string) (*shippingapi.Shipment, error) {
	stub.returnRequest, stub.idempotencyKey, stub.requestID = request, key, requestID
	return stub.returnShipment, stub.err
}
func (stub *shippingClientStub) CancelShipment(context.Context, string, string, string) (*shippingapi.Shipment, error) {
	return stub.shipment, stub.err
}
func (stub *shippingClientStub) GetShipmentByOrder(context.Context, uint, string) (*shippingapi.Shipment, error) {
	return stub.shipment, stub.err
}
func (stub *shippingClientStub) GetTimeline(context.Context, string, string) ([]shippingapi.ShipmentEvent, error) {
	return nil, stub.err
}
func (stub *shippingClientStub) TrackingURL(tracking string) string {
	return "https://pehlione-shipping.com/track/" + tracking
}
func (stub *shippingClientStub) QRCodeURL(tracking string) string {
	return "https://pehlione-shipping.com/qr/" + tracking
}

func TestShippingHandoverUsesSnapshotsAndUpdatesOrderAtomically(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	stub := &shippingClientStub{shipment: &shippingapi.Shipment{ShipmentID: "shp_gate", HandoverCode: handoverCode(order), OrderID: fmt.Sprint(order.ID), TrackingNumber: "TRK-GATE", ShipmentType: "outbound", Status: "created", StatusLabel: "Created"}}
	service := NewShippingService(database, stub)

	cache, err := service.Handover(t.Context(), order.ID, "request-gate")
	if err != nil {
		t.Fatal(err)
	}
	if stub.idempotencyKey != "shipment-order-"+fmt.Sprint(order.ID) || stub.requestID != "request-gate" {
		t.Fatalf("integration headers were not propagated: %q %q", stub.idempotencyKey, stub.requestID)
	}
	if stub.createdRequest.OrderID != fmt.Sprint(order.ID) || stub.createdRequest.CustomerID != fmt.Sprint(order.UserID) || stub.createdRequest.HandoverCode != handoverCode(order) || stub.createdRequest.Recipient.Street != "Snapshot Street" || len(stub.createdRequest.Items) != 1 {
		t.Fatalf("shipment request did not use immutable order snapshots: %+v", stub.createdRequest)
	}
	if cache.TrackingNumber != "TRK-GATE" || cache.HandoverCode != handoverCode(order) {
		t.Fatalf("shipment cache missing: %+v", cache)
	}
	callback := ShippingCallback{
		EventID:    "handover-cache-callback",
		ShipmentID: "shp_gate", OrderID: fmt.Sprint(order.ID), TrackingNumber: "TRK-GATE",
		ShipmentType: "outbound", Status: "in_transit", StatusLabel: "In transit", OccurredAt: time.Now().UTC(),
	}
	if err := service.ApplyCallback(t.Context(), callback); err != nil {
		t.Fatalf("apply shipping callback: %v", err)
	}
	cache, err = service.CachedOrderShipment(t.Context(), order.ID)
	if err != nil || cache == nil || cache.HandoverCode != handoverCode(order) {
		t.Fatalf("callback must preserve the handover code: cache=%+v err=%v", cache, err)
	}
	if err := database.First(&order, order.ID).Error; err != nil || order.Status != models.OrderStatusShipped {
		t.Fatalf("order was not marked shipped: %v %s", err, order.Status)
	}

	// A retry after the successful handover returns the same cached shipment
	// without creating another shipment in Shipping Service.
	replayed, err := service.Handover(t.Context(), order.ID, "request-retry")
	if err != nil || replayed == nil || replayed.ShipmentID != cache.ShipmentID {
		t.Fatalf("duplicate handover was not replayed safely: shipment=%+v err=%v", replayed, err)
	}
}

func TestShippingHandoverFailureLeavesOrderAndCacheUnchanged(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	service := NewShippingService(database, &shippingClientStub{err: shippingapi.ErrUnavailable})

	if _, err := service.Handover(t.Context(), order.ID, "request-failure"); !errors.Is(err, shippingapi.ErrUnavailable) {
		t.Fatalf("unexpected handover error: %v", err)
	}
	if err := database.First(&order, order.ID).Error; err != nil || order.Status != models.OrderStatusReadyForShipping {
		t.Fatalf("failed handover changed order: %v %s", err, order.Status)
	}
	var count int64
	database.Model(&models.OrderShipment{}).Where("order_id = ?", order.ID).Count(&count)
	if count != 0 {
		t.Fatalf("failed handover created %d shipment cache rows", count)
	}
}

func TestShippingHandoverRejectsUnpaidOrder(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	if err := database.Model(&models.Payment{}).Where("order_id = ?", order.ID).Update("status", models.PaymentStatusFailed).Error; err != nil {
		t.Fatal(err)
	}
	service := NewShippingService(database, &shippingClientStub{})
	if _, err := service.Handover(t.Context(), order.ID, "request-unpaid"); !errors.Is(err, ErrShippingPaymentNotReady) {
		t.Fatalf("expected unpaid order to be rejected, got %v", err)
	}
}

func TestShippingCallbackIsIdempotentAndUpdatesCache(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	eventID := fmt.Sprintf("shipping-event-%d-%d", order.ID, time.Now().UnixNano())
	t.Cleanup(func() { database.Unscoped().Where("event_id = ?", eventID).Delete(&models.ShippingEventReceipt{}) })
	stops := 4
	callback := ShippingCallback{EventID: eventID, ShipmentID: "shp_callback", OrderID: fmt.Sprint(order.ID), TrackingNumber: "TRK-CALLBACK", ShipmentType: "outbound", Status: "out_for_delivery", StatusLabel: "Out for delivery", RemainingStops: &stops, OccurredAt: time.Now().UTC()}
	service := NewShippingService(database, &shippingClientStub{})

	if err := service.ApplyCallback(t.Context(), callback); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyCallback(t.Context(), callback); err != nil {
		t.Fatalf("duplicate callback failed: %v", err)
	}
	var receipts int64
	database.Model(&models.ShippingEventReceipt{}).Where("event_id = ?", eventID).Count(&receipts)
	var notifications int64
	database.Model(&models.Notification{}).Where("shipping_event_id = ?", eventID).Count(&notifications)
	cache, err := service.CachedOrderShipment(t.Context(), order.ID)
	if err != nil || receipts != 1 || notifications != 1 || cache == nil || cache.Status != "out_for_delivery" || cache.RemainingStops == nil || *cache.RemainingStops != 4 {
		t.Fatalf("callback was not idempotently cached/notified: receipts=%d notifications=%d cache=%+v err=%v", receipts, notifications, cache, err)
	}
}

func TestShippingStopsCallbackUpdatesCacheWithoutNotification(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	eventID := fmt.Sprintf("shipping-stops-%d-%d", order.ID, time.Now().UnixNano())
	stops := 10
	callback := ShippingCallback{EventID: eventID, EventType: "stops_updated", ShipmentID: "shp_stops", OrderID: fmt.Sprint(order.ID), TrackingNumber: "TRK-STOPS", ShipmentType: "outbound", Status: "out_for_delivery", StatusLabel: "Out for delivery", RemainingStops: &stops, OccurredAt: time.Now().UTC()}
	service := NewShippingService(database, &shippingClientStub{})
	if err := service.ApplyCallback(t.Context(), callback); err != nil {
		t.Fatal(err)
	}

	var notifications int64
	database.Model(&models.Notification{}).Where("shipping_event_id = ?", eventID).Count(&notifications)
	cache, err := service.CachedOrderShipment(t.Context(), order.ID)
	if err != nil || notifications != 0 || cache == nil || cache.RemainingStops == nil || *cache.RemainingStops != 10 {
		t.Fatalf("stops callback result: notifications=%d cache=%+v err=%v", notifications, cache, err)
	}
}

func TestCustomerDeliveryConfirmationAndReturnAreIdempotent(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	outbound := &shippingapi.Shipment{ShipmentID: "shp_outbound", OrderID: fmt.Sprint(order.ID), TrackingNumber: "PHE-DE-20260906-ABC123", ShipmentType: "outbound", Status: "delivered", StatusLabel: "Delivered"}
	returnShipment := &shippingapi.Shipment{ShipmentID: "shp_return", OrderID: fmt.Sprint(order.ID), TrackingNumber: "RET-DE-20260906-ABC123", ShipmentType: "return", Status: "return_requested", StatusLabel: "Return requested"}
	stub := &shippingClientStub{shipment: outbound, returnShipment: returnShipment}
	service := NewShippingService(database, stub)

	if err := service.ConfirmDelivery(t.Context(), order.UserID, order.ID, "request-delivery"); err != nil {
		t.Fatalf("confirm delivery: %v", err)
	}
	if err := service.ConfirmDelivery(t.Context(), order.UserID, order.ID, "request-delivery-retry"); err != nil {
		t.Fatalf("repeat confirmation: %v", err)
	}
	var confirmed models.Order
	if err := database.First(&confirmed, order.ID).Error; err != nil || confirmed.CustomerReceivedAt == nil {
		t.Fatalf("delivery confirmation was not persisted: %v %+v", err, confirmed.CustomerReceivedAt)
	}

	items := []ReturnItemInput{{OrderItemID: 0, Quantity: 1}}
	var orderItem models.OrderItem
	if err := database.Where("order_id = ?", order.ID).First(&orderItem).Error; err != nil {
		t.Fatal(err)
	}
	items[0].OrderItemID = orderItem.ID
	first, err := service.RequestReturn(t.Context(), order.UserID, order.ID, models.ReturnReasonDefective, "does not start", items, "request-return")
	if err != nil {
		t.Fatalf("request return: %v", err)
	}
	second, err := service.RequestReturn(t.Context(), order.UserID, order.ID, models.ReturnReasonDefective, "does not start", items, "request-return-retry")
	if err != nil {
		t.Fatalf("repeat return request: %v", err)
	}
	if first.ID == 0 || first.ID != second.ID || stub.returnRequest.OriginalShipmentID != outbound.ShipmentID || stub.idempotencyKey != "return-order-"+fmt.Sprint(order.ID) {
		t.Fatalf("return request was not idempotent: first=%+v second=%+v request=%+v key=%q", first, second, stub.returnRequest, stub.idempotencyKey)
	}
	var count int64
	if err := database.Model(&models.ReturnRequest{}).Where("order_id = ?", order.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected one return request, got %d: %v", count, err)
	}
}

func createShippingOrderFixture(t *testing.T, database *gorm.DB, fixture checkoutFixture) models.Order {
	t.Helper()
	order := models.Order{UserID: fixture.users[0].ID, Status: models.OrderStatusReadyForShipping, OrderNumber: fmt.Sprintf("SHIP-%d", time.Now().UnixNano()), IdempotencyKey: fmt.Sprintf("%064x", time.Now().UnixNano()), PaymentMethod: models.PaymentMethodCreditCard, Currency: "EUR", TotalCents: fixture.products[0].PriceCents}
	if err := database.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	address := models.OrderAddress{OrderID: order.ID, Type: models.AddressTypeShipping, FirstName: "Snapshot", LastName: "Customer", Street: "Snapshot Street", HouseNumber: "7", PostalCode: "35037", City: "Marburg", CountryCode: "DE", Phone: "+491234567"}
	item := models.OrderItem{OrderID: order.ID, ProductID: fixture.products[0].ID, ProductName: fixture.products[0].Name, ProductSKU: "SKU-GATE", UnitPriceCents: fixture.products[0].PriceCents, Quantity: 1, SubtotalCents: fixture.products[0].PriceCents}
	if err := database.Create(&address).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	payment := models.Payment{OrderID: order.ID, UserID: order.UserID, Provider: "card", Method: models.PaymentMethodCreditCard, ProviderPaymentID: fmt.Sprintf("ship-pay-%d", time.Now().UnixNano()), Status: models.PaymentStatusPaid, AmountCents: order.TotalCents, Currency: "EUR", IdempotencyKey: fmt.Sprintf("%064x", time.Now().UnixNano()+1)}
	if err := database.Create(&payment).Error; err != nil {
		t.Fatal(err)
	}
	return order
}

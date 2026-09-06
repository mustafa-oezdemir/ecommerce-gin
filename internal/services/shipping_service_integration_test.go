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
	idempotencyKey string
	requestID      string
	shipment       *shippingapi.Shipment
	err            error
}

func (stub *shippingClientStub) CreateShipment(_ context.Context, request shippingapi.CreateShipmentRequest, key, requestID string) (*shippingapi.Shipment, error) {
	stub.createdRequest, stub.idempotencyKey, stub.requestID = request, key, requestID
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

func TestShippingHandoverUsesSnapshotsAndUpdatesOrderAtomically(t *testing.T) {
	database := checkoutIntegrationDatabase(t)
	fixture := newCheckoutFixture(t, database, 1, 1)
	order := createShippingOrderFixture(t, database, fixture)
	stub := &shippingClientStub{shipment: &shippingapi.Shipment{ShipmentID: "shp_gate", OrderID: fmt.Sprint(order.ID), TrackingNumber: "TRK-GATE", ShipmentType: "outbound", Status: "created", StatusLabel: "Created"}}
	service := NewShippingService(database, stub)

	cache, err := service.Handover(t.Context(), order.ID, "request-gate")
	if err != nil {
		t.Fatal(err)
	}
	if stub.idempotencyKey != "shipment-order-"+fmt.Sprint(order.ID) || stub.requestID != "request-gate" {
		t.Fatalf("integration headers were not propagated: %q %q", stub.idempotencyKey, stub.requestID)
	}
	if stub.createdRequest.OrderID != fmt.Sprint(order.ID) || stub.createdRequest.CustomerID != fmt.Sprint(order.UserID) || stub.createdRequest.Recipient.Street != "Snapshot Street" || len(stub.createdRequest.Items) != 1 {
		t.Fatalf("shipment request did not use immutable order snapshots: %+v", stub.createdRequest)
	}
	if cache.TrackingNumber != "TRK-GATE" {
		t.Fatalf("shipment cache missing: %+v", cache)
	}
	if err := database.First(&order, order.ID).Error; err != nil || order.Status != models.OrderStatusShipped {
		t.Fatalf("order was not marked shipped: %v %s", err, order.Status)
	}

	// A retry after the successful handover must not call Shipping again.
	_, err = service.Handover(t.Context(), order.ID, "request-retry")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("duplicate handover was not rejected: %v", err)
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
	if err := database.First(&order, order.ID).Error; err != nil || order.Status != models.OrderStatusProcessing {
		t.Fatalf("failed handover changed order: %v %s", err, order.Status)
	}
	var count int64
	database.Model(&models.OrderShipment{}).Where("order_id = ?", order.ID).Count(&count)
	if count != 0 {
		t.Fatalf("failed handover created %d shipment cache rows", count)
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
	cache, err := service.CachedOrderShipment(t.Context(), order.ID)
	if err != nil || receipts != 1 || cache == nil || cache.Status != "out_for_delivery" || cache.RemainingStops == nil || *cache.RemainingStops != 4 {
		t.Fatalf("callback was not idempotently cached: receipts=%d cache=%+v err=%v", receipts, cache, err)
	}
}

func createShippingOrderFixture(t *testing.T, database *gorm.DB, fixture checkoutFixture) models.Order {
	t.Helper()
	order := models.Order{UserID: fixture.users[0].ID, Status: models.OrderStatusProcessing, OrderNumber: fmt.Sprintf("SHIP-%d", time.Now().UnixNano()), IdempotencyKey: fmt.Sprintf("%064x", time.Now().UnixNano()), PaymentMethod: models.PaymentMethodCreditCard, Currency: "EUR", TotalCents: fixture.products[0].PriceCents}
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
	return order
}

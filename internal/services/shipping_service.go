package services

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	shippingapi "github.com/mustafa-oezdemir/ecommerce-gin/internal/shipping"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrShippingDisabled        = errors.New("shipping integration is not configured")
	ErrShippingAddressAbsent   = errors.New("order shipping address is unavailable")
	ErrInvalidShippingCallback = errors.New("invalid shipping callback")
)

type ShippingService struct {
	database *gorm.DB
	client   shippingapi.Client
}

type ShippingCallback struct {
	EventID           string                         `json:"event_id"`
	ShipmentID        string                         `json:"shipment_id"`
	OrderID           string                         `json:"order_id"`
	TrackingNumber    string                         `json:"tracking_number"`
	ShipmentType      string                         `json:"shipment_type"`
	Status            string                         `json:"status"`
	StatusLabel       string                         `json:"status_label"`
	RemainingStops    *int                           `json:"remaining_stops,omitempty"`
	EstimatedDelivery *shippingapi.EstimatedDelivery `json:"estimated_delivery,omitempty"`
	OccurredAt        time.Time                      `json:"occurred_at"`
}

func NewShippingService(database *gorm.DB, client shippingapi.Client) *ShippingService {
	if database == nil {
		panic("services: database is required")
	}
	return &ShippingService{database: database, client: client}
}

func (service *ShippingService) Enabled() bool { return service.client != nil }

func (service *ShippingService) TrackingURL(trackingNumber string) string {
	if service.client == nil {
		return ""
	}
	return service.client.TrackingURL(trackingNumber)
}

func (service *ShippingService) Timeline(ctx context.Context, trackingNumber, requestID string) ([]shippingapi.ShipmentEvent, error) {
	if service.client == nil {
		return nil, ErrShippingDisabled
	}
	return service.client.GetTimeline(ctx, trackingNumber, requestID)
}

func (service *ShippingService) Handover(ctx context.Context, orderID uint, requestID string) (*models.OrderShipment, error) {
	if service.client == nil {
		return nil, ErrShippingDisabled
	}
	var order models.Order
	if err := service.database.WithContext(ctx).Preload("Items").Preload("Addresses").First(&order, orderID).Error; err != nil {
		return nil, err
	}
	if order.Status != models.OrderStatusProcessing {
		return nil, ErrInvalidTransition
	}
	address, ok := orderShippingAddress(order.Addresses)
	if !ok {
		return nil, ErrShippingAddressAbsent
	}
	items := make([]shippingapi.Item, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, shippingapi.Item{
			ProductID: strconv.FormatUint(uint64(item.ProductID), 10),
			Name:      item.ProductName,
			SKU:       item.ProductSKU,
			Quantity:  item.Quantity,
		})
	}
	shipment, err := service.client.CreateShipment(ctx, shippingapi.CreateShipmentRequest{
		OrderID:      strconv.FormatUint(uint64(order.ID), 10),
		CustomerID:   strconv.FormatUint(uint64(order.UserID), 10),
		Recipient:    shippingAddress(address),
		Items:        items,
		Carrier:      "NordShip",
		ServiceLevel: "standard",
	}, "shipment-order-"+strconv.FormatUint(uint64(order.ID), 10), requestID)
	if err != nil {
		return nil, err
	}
	cache := shipmentCache(order.ID, shipment, "")
	err = service.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := upsertShipment(transaction, cache); err != nil {
			return err
		}
		result := transaction.Model(&models.Order{}).Where("id = ? AND status = ?", order.ID, models.OrderStatusProcessing).Update("status", models.OrderStatusShipped)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var current models.Order
			if err := transaction.Select("status").First(&current, order.ID).Error; err != nil {
				return err
			}
			if current.Status != models.OrderStatusShipped {
				return ErrInvalidTransition
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cache, nil
}

func (service *ShippingService) RefreshOrderShipment(ctx context.Context, orderID uint, requestID string) (*models.OrderShipment, error) {
	if service.client == nil {
		return nil, ErrShippingDisabled
	}
	shipment, err := service.client.GetShipmentByOrder(ctx, orderID, requestID)
	if err != nil {
		return nil, err
	}
	cache := shipmentCache(orderID, shipment, "")
	if err := upsertShipment(service.database.WithContext(ctx), cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

func (service *ShippingService) ApplyCallback(ctx context.Context, callback ShippingCallback) error {
	if callback.EventID == "" || callback.ShipmentID == "" || callback.OrderID == "" || callback.TrackingNumber == "" || callback.Status == "" || callback.StatusLabel == "" || callback.OccurredAt.IsZero() || (callback.ShipmentType != "outbound" && callback.ShipmentType != "return") {
		return ErrInvalidShippingCallback
	}
	orderID, err := strconv.ParseUint(callback.OrderID, 10, 64)
	if err != nil || orderID == 0 {
		return ErrInvalidShippingCallback
	}
	return service.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		receipt := models.ShippingEventReceipt{EventID: callback.EventID}
		if err := transaction.Create(&receipt).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return nil
			}
			return err
		}
		var order models.Order
		if err := transaction.Select("id").First(&order, uint(orderID)).Error; err != nil {
			return err
		}
		cache := models.OrderShipment{
			OrderID: uint(orderID), ShipmentID: callback.ShipmentID, TrackingNumber: callback.TrackingNumber,
			ShipmentType: callback.ShipmentType, Status: callback.Status, StatusLabel: callback.StatusLabel,
			RemainingStops: callback.RemainingStops, LastShippingEvent: callback.EventID,
		}
		if callback.EstimatedDelivery != nil {
			cache.EstimatedFrom, cache.EstimatedUntil = callback.EstimatedDelivery.From, callback.EstimatedDelivery.Until
		}
		return upsertShipment(transaction, cache)
	})
}

func (service *ShippingService) CachedOrderShipment(ctx context.Context, orderID uint) (*models.OrderShipment, error) {
	var shipment models.OrderShipment
	if err := service.database.WithContext(ctx).Where("order_id = ? AND shipment_type = ?", orderID, "outbound").First(&shipment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &shipment, nil
}

func orderShippingAddress(addresses []models.OrderAddress) (models.OrderAddress, bool) {
	for _, address := range addresses {
		if address.Type == models.AddressTypeShipping {
			return address, true
		}
	}
	return models.OrderAddress{}, false
}

func shippingAddress(address models.OrderAddress) shippingapi.Address {
	return shippingapi.Address{
		FirstName: address.FirstName, LastName: address.LastName, Company: address.Company, Street: address.Street,
		HouseNumber: address.HouseNumber, AddressLine2: address.AddressLine2, PostalCode: address.PostalCode,
		City: address.City, State: address.State, CountryCode: address.CountryCode, Phone: address.Phone,
	}
}

func shipmentCache(orderID uint, shipment *shippingapi.Shipment, eventID string) models.OrderShipment {
	cache := models.OrderShipment{
		OrderID: orderID, ShipmentID: shipment.ShipmentID, TrackingNumber: shipment.TrackingNumber, ShipmentType: shipment.ShipmentType,
		Status: shipment.Status, StatusLabel: shipment.StatusLabel, RemainingStops: shipment.RemainingStops, LastShippingEvent: eventID,
	}
	if shipment.EstimatedDelivery != nil {
		cache.EstimatedFrom, cache.EstimatedUntil = shipment.EstimatedDelivery.From, shipment.EstimatedDelivery.Until
	}
	return cache
}

func upsertShipment(database *gorm.DB, shipment models.OrderShipment) error {
	return database.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "order_id"}, {Name: "shipment_type"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"shipment_id", "tracking_number", "shipment_type", "status", "status_label", "remaining_stops",
			"estimated_from", "estimated_until", "last_shipping_event", "updated_at",
		}),
	}).Create(&shipment).Error
}

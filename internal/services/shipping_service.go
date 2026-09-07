package services

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	shippingapi "github.com/mustafa-oezdemir/ecommerce-gin/internal/shipping"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrShippingDisabled        = errors.New("shipping integration is not configured")
	ErrShippingAddressAbsent   = errors.New("order shipping address is unavailable")
	ErrShippingPaymentNotReady = errors.New("order payment is not ready for shipping")
	ErrInvalidShippingCallback = errors.New("invalid shipping callback")
	ErrDeliveryNotConfirmed    = errors.New("shipment has not been delivered")
	ErrInvalidReturnRequest    = errors.New("invalid return request")
	ErrReturnNotAtWarehouse    = errors.New("return shipment has not reached the warehouse")
	ErrReturnAlreadyConfirmed  = errors.New("warehouse return was already confirmed")
)

type ShippingService struct {
	database *gorm.DB
	client   shippingapi.Client
	mailer   *MailService
}

type ShippingCallback struct {
	EventID           string                         `json:"event_id"`
	EventType         string                         `json:"event_type,omitempty"`
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

type ReturnItemInput struct {
	OrderItemID uint
	Quantity    int
}

func NewShippingService(database *gorm.DB, client shippingapi.Client, mailers ...*MailService) *ShippingService {
	if database == nil {
		panic("services: database is required")
	}
	var mailer *MailService
	if len(mailers) > 0 {
		mailer = mailers[0]
	}
	return &ShippingService{database: database, client: client, mailer: mailer}
}

func (service *ShippingService) Enabled() bool { return service.client != nil }

func (service *ShippingService) TrackingURL(trackingNumber string) string {
	if service.client == nil {
		return ""
	}
	return service.client.TrackingURL(trackingNumber)
}

func (service *ShippingService) QRCodeURL(trackingNumber string) string {
	if service.client == nil {
		return ""
	}
	return service.client.QRCodeURL(trackingNumber)
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
	if err := service.database.WithContext(ctx).Preload("Items").Preload("Addresses").Preload("Payment").First(&order, orderID).Error; err != nil {
		return nil, err
	}
	if order.Status == models.OrderStatusShipped {
		if cached, err := service.CachedOrderShipment(ctx, orderID); err != nil || cached != nil {
			return cached, err
		}
	}
	if order.Status != models.OrderStatusReadyForShipping {
		return nil, ErrInvalidTransition
	}
	if order.Payment.ID == 0 || (order.Payment.Status != models.PaymentStatusPaid && order.Payment.Status != models.PaymentStatusAuthorized) {
		return nil, ErrShippingPaymentNotReady
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
		HandoverCode: handoverCode(order),
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
		result := transaction.Model(&models.Order{}).Where("id = ? AND status = ?", order.ID, models.OrderStatusReadyForShipping).Update("status", models.OrderStatusShipped)
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

func (service *ShippingService) ConfirmDelivery(ctx context.Context, userID, orderID uint, requestID string) error {
	if service.client == nil {
		return ErrShippingDisabled
	}
	if userID == 0 || orderID == 0 {
		return ErrInvalidReturnRequest
	}
	var owned models.Order
	if err := service.database.WithContext(ctx).Select("id").Where("id = ? AND user_id = ?", orderID, userID).First(&owned).Error; err != nil {
		return err
	}
	shipment, err := service.client.GetShipmentByOrder(ctx, orderID, requestID)
	if err != nil {
		return err
	}
	if shipment == nil || shipment.Status != "delivered" {
		return ErrDeliveryNotConfirmed
	}
	if err := upsertShipment(service.database.WithContext(ctx), shipmentCache(orderID, shipment, "")); err != nil {
		return err
	}
	now := time.Now().UTC()
	result := service.database.WithContext(ctx).Model(&models.Order{}).
		Where("id = ? AND user_id = ? AND customer_received_at IS NULL", orderID, userID).
		Update("customer_received_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var order models.Order
		if err := service.database.WithContext(ctx).Select("customer_received_at").Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
			return err
		}
	}
	return nil
}

func (service *ShippingService) RequestReturn(ctx context.Context, userID, orderID uint, reason models.ReturnReason, note string, items []ReturnItemInput, requestID string) (*models.ReturnRequest, error) {
	if service.client == nil {
		return nil, ErrShippingDisabled
	}
	if userID == 0 || orderID == 0 || !reason.Valid() || len([]rune(note)) > 1000 || len(items) == 0 {
		return nil, ErrInvalidReturnRequest
	}
	var order models.Order
	if err := service.database.WithContext(ctx).Preload("Items").Preload("Addresses").Preload("ReturnRequest").First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		return nil, err
	}
	if order.ReturnRequest != nil {
		return order.ReturnRequest, nil
	}
	outbound, err := service.client.GetShipmentByOrder(ctx, orderID, requestID)
	if err != nil {
		return nil, err
	}
	if outbound == nil || outbound.Status != "delivered" {
		return nil, ErrDeliveryNotConfirmed
	}
	if outbound.DeliveredAt != nil && time.Since(outbound.DeliveredAt.UTC()) > 30*24*time.Hour {
		return nil, ErrInvalidReturnRequest
	}
	address, ok := orderShippingAddress(order.Addresses)
	if !ok {
		return nil, ErrShippingAddressAbsent
	}
	orderItems := make(map[uint]models.OrderItem, len(order.Items))
	for _, item := range order.Items {
		orderItems[item.ID] = item
	}
	returnItems := make([]models.ReturnItem, 0, len(items))
	shipmentItems := make([]shippingapi.Item, 0, len(items))
	selectedItems := make(map[uint]struct{}, len(items))
	for _, item := range items {
		orderItem, found := orderItems[item.OrderItemID]
		if !found || item.Quantity < 1 || item.Quantity > orderItem.Quantity {
			return nil, ErrInvalidReturnRequest
		}
		if _, duplicate := selectedItems[item.OrderItemID]; duplicate {
			return nil, ErrInvalidReturnRequest
		}
		selectedItems[item.OrderItemID] = struct{}{}
		returnItems = append(returnItems, models.ReturnItem{OrderItemID: orderItem.ID, ProductID: orderItem.ProductID, Quantity: item.Quantity})
		shipmentItems = append(shipmentItems, shippingapi.Item{ProductID: strconv.FormatUint(uint64(orderItem.ProductID), 10), Name: orderItem.ProductName, SKU: orderItem.ProductSKU, Quantity: item.Quantity})
	}
	shipment, err := service.client.CreateReturn(ctx, shippingapi.CreateReturnRequest{
		OriginalShipmentID: outbound.ShipmentID,
		OrderID:            strconv.FormatUint(uint64(order.ID), 10),
		CustomerID:         strconv.FormatUint(uint64(order.UserID), 10),
		Sender:             shippingAddress(address),
		Items:              shipmentItems,
	}, "return-order-"+strconv.FormatUint(uint64(order.ID), 10), requestID)
	if err != nil {
		return nil, err
	}
	if shipment == nil || shipment.ShipmentID == "" || shipment.TrackingNumber == "" || shipment.ShipmentType != "return" {
		return nil, ErrInvalidReturnRequest
	}
	returnRequest := models.ReturnRequest{
		OrderID: order.ID, UserID: order.UserID, Reason: reason, Note: strings.TrimSpace(note),
		OriginalShipmentID: outbound.ShipmentID, ReturnShipmentID: shipment.ShipmentID,
		ReturnTrackingNumber: shipment.TrackingNumber, Status: shipment.Status,
	}
	err = service.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&returnRequest).Error; err != nil {
			return err
		}
		if returnRequest.ID == 0 {
			if err := transaction.Where("order_id = ?", order.ID).First(&returnRequest).Error; err != nil {
				return err
			}
		} else {
			for index := range returnItems {
				returnItems[index].ReturnRequestID = returnRequest.ID
			}
			if err := transaction.Create(&returnItems).Error; err != nil {
				return err
			}
		}
		return upsertShipment(transaction, shipmentCache(order.ID, shipment, ""))
	})
	if err != nil {
		return nil, err
	}
	return &returnRequest, nil
}

func (service *ShippingService) CancelOrder(ctx context.Context, userID, orderID uint, requestID string) error {
	if userID == 0 || orderID == 0 {
		return ErrInvalidTransition
	}
	var order models.Order
	if err := service.database.WithContext(ctx).Preload("ReturnRequest").First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		return err
	}
	if order.ReturnRequest != nil {
		return ErrInvalidTransition
	}
	switch order.Status {
	case models.OrderStatusCancelled:
		return nil
	case models.OrderStatusPaid, models.OrderStatusPreparing, models.OrderStatusReadyForShipping, models.OrderStatusProcessing:
		return service.database.WithContext(ctx).Model(&models.Order{}).Where("id = ? AND user_id = ? AND status IN ?", orderID, userID, []models.OrderStatus{models.OrderStatusPaid, models.OrderStatusPreparing, models.OrderStatusReadyForShipping, models.OrderStatusProcessing}).Update("status", models.OrderStatusCancelled).Error
	case models.OrderStatusShipped:
		if service.client == nil {
			return ErrShippingDisabled
		}
		shipment, err := service.client.GetShipmentByOrder(ctx, orderID, requestID)
		if err != nil {
			return err
		}
		if shipment == nil || shipment.Status == "delivered" {
			return ErrInvalidTransition
		}
		cancelledShipment, err := service.client.CancelShipment(ctx, shipment.ShipmentID, "cancel-shipment-order-"+strconv.FormatUint(uint64(orderID), 10), requestID)
		if err != nil {
			return err
		}
		if cancelledShipment == nil {
			return ErrInvalidTransition
		}
		if err := upsertShipment(service.database.WithContext(ctx), shipmentCache(orderID, cancelledShipment, "")); err != nil {
			return err
		}
		// Once logistics has received the parcel, Shipping owns the return-to-sender
		// lifecycle. The commerce order remains shipped and exposes that read-only state.
		if cancelledShipment.Status != "cancelled" {
			return nil
		}
		result := service.database.WithContext(ctx).Model(&models.Order{}).Where("id = ? AND user_id = ? AND status = ?", orderID, userID, models.OrderStatusShipped).Update("status", models.OrderStatusCancelled)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var current models.Order
			if err := service.database.WithContext(ctx).Select("status").First(&current, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
				return err
			}
			if current.Status != models.OrderStatusCancelled {
				return ErrInvalidTransition
			}
		}
		return nil
	default:
		return ErrInvalidTransition
	}
}

func (service *ShippingService) ConfirmWarehouseReturn(ctx context.Context, orderID uint) error {
	if orderID == 0 {
		return ErrReturnNotAtWarehouse
	}
	return service.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var returnRequest models.ReturnRequest
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", orderID).First(&returnRequest).Error; err != nil {
			return err
		}
		if returnRequest.WarehouseReturnConfirmedAt != nil {
			return ErrReturnAlreadyConfirmed
		}
		var shipment models.OrderShipment
		if err := transaction.Where("order_id = ? AND shipment_type = ?", orderID, "return").First(&shipment).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrReturnNotAtWarehouse
			}
			return err
		}
		if returnRequest.Status != models.ReturnStatusReceivedAtWarehouse || shipment.Status != models.ReturnStatusReceivedAtWarehouse {
			return ErrReturnNotAtWarehouse
		}
		now := time.Now().UTC()
		result := transaction.Model(&models.ReturnRequest{}).
			Where("id = ? AND warehouse_return_confirmed_at IS NULL", returnRequest.ID).
			Update("warehouse_return_confirmed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReturnAlreadyConfirmed
		}
		return nil
	})
}

func (service *ShippingService) ApplyCallback(ctx context.Context, callback ShippingCallback) error {
	if callback.EventID == "" || callback.ShipmentID == "" || callback.OrderID == "" || callback.TrackingNumber == "" || callback.Status == "" || callback.StatusLabel == "" || callback.OccurredAt.IsZero() || (callback.ShipmentType != "outbound" && callback.ShipmentType != "return") {
		return ErrInvalidShippingCallback
	}
	orderID, err := strconv.ParseUint(callback.OrderID, 10, 64)
	if err != nil || orderID == 0 {
		return ErrInvalidShippingCallback
	}
	created := false
	var recipient models.User
	err = service.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		receipt := models.ShippingEventReceipt{EventID: callback.EventID}
		if err := transaction.Create(&receipt).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return nil
			}
			return err
		}
		var order models.Order
		if err := transaction.Select("id", "user_id").First(&order, uint(orderID)).Error; err != nil {
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
		if err := upsertShipment(transaction, cache); err != nil {
			return err
		}
		if callback.ShipmentType == "return" {
			if err := transaction.Model(&models.ReturnRequest{}).Where("return_shipment_id = ?", callback.ShipmentID).Update("status", callback.Status).Error; err != nil {
				return err
			}
		}
		message, notify := shippingNotification(callback, uint(orderID))
		if !notify {
			return nil
		}
		message.UserID = order.UserID
		if err := transaction.Create(&message).Error; err != nil {
			return err
		}
		if err := transaction.First(&recipient, order.UserID).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return err
	}
	if created && service.mailer != nil {
		go service.mailer.SendShipmentUpdate(recipient, uint(orderID), callback.Status, callback.StatusLabel)
	}
	return nil
}

func shippingNotification(callback ShippingCallback, orderID uint) (models.Notification, bool) {
	// Legacy senders did not include event_type. When it is present, only
	// lifecycle transitions create notifications; ETA/stops updates stay on
	// the read-only tracking screen.
	if callback.EventType != "" && callback.EventType != "status_changed" {
		return models.Notification{}, false
	}
	titles := map[string]string{
		"awaiting_receipt":             "Your order was handed to shipping",
		"received_by_shipping":         "Shipment received by logistics",
		"shipment_prepared":            "Your shipment was prepared",
		"in_transit":                   "Your shipment is on the way",
		"out_for_delivery":             "Out for delivery",
		"delivered":                    "Your order has been delivered",
		"delivery_failed":              "Delivery attempt failed",
		"return_authorized":            "Your return was authorized",
		"return_in_transit":            "Your return is on the way",
		"return_received_at_warehouse": "Return received at warehouse",
		"return_completed":             "Return completed",
	}
	title, ok := titles[callback.Status]
	if !ok {
		return models.Notification{}, false
	}
	message := fmt.Sprintf("Order #%d shipping status is now %s.", orderID, callback.StatusLabel)
	if callback.Status == "out_for_delivery" && callback.RemainingStops != nil {
		message = fmt.Sprintf("Order #%d is out for delivery with %d stops remaining.", orderID, *callback.RemainingStops)
	}
	return models.Notification{Type: "shipping_" + callback.Status, Title: title, Message: message, RelatedOrderID: orderID, ShippingEventID: callback.EventID}, true
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

func (service *ShippingService) CachedReturnShipment(ctx context.Context, orderID uint) (*models.OrderShipment, error) {
	var shipment models.OrderShipment
	if err := service.database.WithContext(ctx).Where("order_id = ? AND shipment_type = ?", orderID, "return").First(&shipment).Error; err != nil {
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
		OrderID: orderID, ShipmentID: shipment.ShipmentID, HandoverCode: shipment.HandoverCode, TrackingNumber: shipment.TrackingNumber, ShipmentType: shipment.ShipmentType,
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

// handoverCode derives a stable opaque code from the checkout idempotency key.
// That keeps a retry byte-for-byte equivalent without exposing sequential order IDs.
func handoverCode(order models.Order) string {
	sum := sha256.Sum256([]byte("shipment-handover:" + order.IdempotencyKey))
	return fmt.Sprintf("PHE-HO-DE-%s-%X", order.CreatedAt.UTC().Format("20060102"), sum[:8])
}

package shipping

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appmetrics "github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
)

var (
	ErrUnavailable  = errors.New("shipping service is unavailable")
	ErrTimeout      = errors.New("shipping request timed out")
	ErrUnauthorized = errors.New("shipping service rejected credentials")
	ErrNotFound     = errors.New("shipment was not found")
	ErrConflict     = errors.New("shipping request conflicted with existing data")
)

type responseError struct {
	cause     error
	retryable bool
	message   string
}

func (err *responseError) Error() string {
	if err.message == "" {
		return err.cause.Error()
	}
	return err.cause.Error() + ": " + err.message
}

func (err *responseError) Unwrap() error { return err.cause }

type Address struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Company      string `json:"company,omitempty"`
	Street       string `json:"street"`
	HouseNumber  string `json:"house_number"`
	AddressLine2 string `json:"address_line_2,omitempty"`
	PostalCode   string `json:"postal_code"`
	City         string `json:"city"`
	State        string `json:"state,omitempty"`
	CountryCode  string `json:"country_code"`
	Phone        string `json:"phone,omitempty"`
}

type Item struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	SKU       string `json:"sku,omitempty"`
	Quantity  int    `json:"quantity"`
}

type CreateShipmentRequest struct {
	OrderID      string  `json:"order_id"`
	CustomerID   string  `json:"customer_id"`
	HandoverCode string  `json:"handover_code"`
	Recipient    Address `json:"recipient"`
	Items        []Item  `json:"items"`
	Carrier      string  `json:"carrier"`
	ServiceLevel string  `json:"service_level"`
}

type CreateReturnRequest struct {
	OriginalShipmentID string  `json:"original_shipment_id"`
	OrderID            string  `json:"order_id"`
	CustomerID         string  `json:"customer_id"`
	Sender             Address `json:"sender"`
	Items              []Item  `json:"items"`
}

type EstimatedDelivery struct {
	From  *time.Time `json:"from,omitempty"`
	Until *time.Time `json:"until,omitempty"`
}

type Shipment struct {
	ShipmentID        string             `json:"shipment_id"`
	HandoverCode      string             `json:"handover_code"`
	OrderID           string             `json:"order_id"`
	TrackingNumber    string             `json:"tracking_number"`
	ShipmentType      string             `json:"shipment_type"`
	Status            string             `json:"status"`
	StatusLabel       string             `json:"status_label"`
	RemainingStops    *int               `json:"remaining_stops,omitempty"`
	DeliveredAt       *time.Time         `json:"delivered_at,omitempty"`
	EstimatedDelivery *EstimatedDelivery `json:"estimated_delivery,omitempty"`
	IdempotentReplay  bool               `json:"idempotent_replay"`
}

type ShipmentEvent struct {
	EventID           string             `json:"event_id"`
	Status            string             `json:"status"`
	Title             string             `json:"title"`
	Description       string             `json:"description,omitempty"`
	Location          string             `json:"location,omitempty"`
	RemainingStops    *int               `json:"remaining_stops,omitempty"`
	EstimatedDelivery *EstimatedDelivery `json:"estimated_delivery,omitempty"`
	OccurredAt        time.Time          `json:"occurred_at"`
}

type Client interface {
	CreateShipment(context.Context, CreateShipmentRequest, string, string) (*Shipment, error)
	CreateReturn(context.Context, CreateReturnRequest, string, string) (*Shipment, error)
	CancelShipment(context.Context, string, string, string) (*Shipment, error)
	GetShipmentByOrder(context.Context, uint, string) (*Shipment, error)
	GetTimeline(context.Context, string, string) ([]ShipmentEvent, error)
	TrackingURL(string) string
	QRCodeURL(string) string
}

type HTTPClient struct {
	baseURL    *url.URL
	publicURL  *url.URL
	token      string
	httpClient *http.Client
}

func NewHTTPClient(baseURL, token string, timeout time.Duration, publicBaseURL ...string) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || !validBaseURL(parsed) {
		return nil, fmt.Errorf("invalid shipping service URL")
	}
	if len(token) < 32 {
		return nil, fmt.Errorf("shipping service token must be at least 32 characters")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("shipping request timeout must be positive")
	}
	publicParsed := parsed
	if len(publicBaseURL) > 0 && strings.TrimSpace(publicBaseURL[0]) != "" {
		publicParsed, err = url.Parse(strings.TrimRight(strings.TrimSpace(publicBaseURL[0]), "/"))
		if err != nil || !validBaseURL(publicParsed) {
			return nil, fmt.Errorf("invalid shipping public URL")
		}
	}
	return &HTTPClient{baseURL: parsed, publicURL: publicParsed, token: token, httpClient: &http.Client{Timeout: timeout}}, nil
}

func validBaseURL(parsed *url.URL) bool {
	return parsed != nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func (client *HTTPClient) CreateShipment(ctx context.Context, shipment CreateShipmentRequest, idempotencyKey, requestID string) (*Shipment, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, fmt.Errorf("shipment idempotency key is required")
	}
	var response Shipment
	if err := client.doJSON(ctx, http.MethodPost, "/api/v1/shipments", shipment, &response, idempotencyKey, requestID, true); err != nil {
		return nil, err
	}
	return &response, nil
}

func (client *HTTPClient) CreateReturn(ctx context.Context, shipment CreateReturnRequest, idempotencyKey, requestID string) (*Shipment, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, fmt.Errorf("return idempotency key is required")
	}
	var response Shipment
	if err := client.doJSON(ctx, http.MethodPost, "/api/v1/returns", shipment, &response, idempotencyKey, requestID, true); err != nil {
		return nil, err
	}
	return &response, nil
}

func (client *HTTPClient) CancelShipment(ctx context.Context, shipmentID, idempotencyKey, requestID string) (*Shipment, error) {
	if strings.TrimSpace(shipmentID) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return nil, fmt.Errorf("shipment id and idempotency key are required")
	}
	var response Shipment
	if err := client.doJSON(ctx, http.MethodPost, "/api/v1/shipments/"+url.PathEscape(shipmentID)+"/cancel", nil, &response, idempotencyKey, requestID, true); err != nil {
		return nil, err
	}
	return &response, nil
}

func (client *HTTPClient) GetShipmentByOrder(ctx context.Context, orderID uint, requestID string) (*Shipment, error) {
	if orderID == 0 {
		return nil, ErrNotFound
	}
	var response Shipment
	if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/shipments/order/%d", orderID), nil, &response, "", requestID, true); err != nil {
		return nil, err
	}
	return &response, nil
}

func (client *HTTPClient) TrackingURL(trackingNumber string) string {
	return client.publicURL.String() + "/track/" + url.PathEscape(trackingNumber)
}

func (client *HTTPClient) QRCodeURL(trackingNumber string) string {
	return client.publicURL.String() + "/qr/" + url.PathEscape(trackingNumber)
}

func (client *HTTPClient) GetTimeline(ctx context.Context, trackingNumber, requestID string) ([]ShipmentEvent, error) {
	trackingNumber = strings.TrimSpace(trackingNumber)
	if trackingNumber == "" {
		return nil, ErrNotFound
	}
	var response []ShipmentEvent
	path := "/api/v1/shipments/" + url.PathEscape(trackingNumber) + "/events"
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &response, "", requestID, true); err != nil {
		return nil, err
	}
	return response, nil
}

func (client *HTTPClient) doJSON(ctx context.Context, method, path string, input, output any, idempotencyKey, requestID string, retryable bool) error {
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode shipping request: %w", err)
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, client.baseURL.String()+path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build shipping request: %w", err)
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+client.token)
		request.Header.Set("User-Agent", "ecommerce-gin/1.0")
		request.Header.Set("X-Service-Name", "ecommerce-gin")
		if requestID != "" {
			request.Header.Set("X-Request-ID", requestID)
		}
		if idempotencyKey != "" {
			request.Header.Set("Idempotency-Key", idempotencyKey)
		}
		if input != nil {
			request.Header.Set("Content-Type", "application/json")
		}

		startedAt := time.Now()
		response, err := client.httpClient.Do(request)
		observeShippingRequest(method, path, response, err, time.Since(startedAt))
		if err != nil {
			if retryable && attempt < 2 && isTemporary(err) && ctx.Err() == nil {
				if err := waitForRetry(ctx, attempt); err != nil {
					return err
				}
				continue
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("%w: %v", ErrTimeout, err)
			}
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		err = decodeResponse(response, output)
		if closeErr := response.Body.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err == nil || !retryable || !isRetryableResponse(err) || attempt == 2 || ctx.Err() != nil {
			return err
		}
		if err := waitForRetry(ctx, attempt); err != nil {
			return err
		}
	}
	return ErrUnavailable
}

func observeShippingRequest(method, path string, response *http.Response, requestErr error, duration time.Duration) {
	appMetrics := appmetrics.Default()
	if appMetrics == nil {
		return
	}
	route := shippingMetricRoute(path)
	status := "transport_error"
	if response != nil {
		status = strconv.Itoa(response.StatusCode)
	}
	appMetrics.ShippingAPIRequests.WithLabelValues(method, route, status).Inc()
	appMetrics.ShippingAPIDuration.WithLabelValues(method, route).Observe(duration.Seconds())
	if requestErr != nil || response == nil || response.StatusCode >= http.StatusBadRequest {
		kind := "http"
		if requestErr != nil || response == nil {
			kind = "transport"
		}
		appMetrics.ShippingAPIErrors.WithLabelValues(method, route, kind).Inc()
	}
}

func shippingMetricRoute(path string) string {
	switch {
	case strings.HasSuffix(path, "/events"):
		return "/api/v1/shipments/:trackingNumber/events"
	case strings.HasPrefix(path, "/api/v1/shipments/order/"):
		return "/api/v1/shipments/order/:orderID"
	case strings.HasSuffix(path, "/cancel"):
		return "/api/v1/shipments/:id/cancel"
	case strings.HasPrefix(path, "/api/v1/shipments/"):
		return "/api/v1/shipments/:trackingNumber"
	case strings.HasPrefix(path, "/api/v1/returns/"):
		return "/api/v1/returns/:trackingNumber"
	default:
		return path
	}
}

func decodeResponse(response *http.Response, output any) error {
	defer io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return &responseError{cause: ErrUnavailable, message: "invalid response"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Error.Message)
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &responseError{cause: ErrUnauthorized, message: message}
		case http.StatusNotFound:
			return &responseError{cause: ErrNotFound, message: message}
		case http.StatusConflict:
			return &responseError{cause: ErrConflict, message: message}
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return &responseError{cause: ErrUnavailable, retryable: true, message: message}
		default:
			return &responseError{cause: ErrUnavailable, message: message}
		}
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return &responseError{cause: ErrUnavailable, message: "invalid response data"}
	}
	return nil
}

func waitForRetry(ctx context.Context, attempt int) error {
	delay := 100 * time.Millisecond * time.Duration(1<<attempt)
	if jitter, err := rand.Int(rand.Reader, big.NewInt(int64(delay/2)+1)); err == nil {
		delay += time.Duration(jitter.Int64())
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableResponse(err error) bool {
	var responseErr *responseError
	return errors.As(err, &responseErr) && responseErr.retryable
}

func isTemporary(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}

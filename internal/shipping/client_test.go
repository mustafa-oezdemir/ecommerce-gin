package shipping

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateShipmentAuthenticatesPropagatesHeadersAndRetries(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer 01234567890123456789012345678901" {
			t.Fatal("missing service authorization")
		}
		if request.Header.Get("Idempotency-Key") != "shipment-order-17" || request.Header.Get("X-Request-ID") != "request-17" {
			t.Fatal("shipping headers were not propagated")
		}
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(`{"success":false,"error":{"message":"retry"}}`))
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]any{"shipment_id": "shp_123", "tracking_number": "NS-DE-20260906-ABC123", "shipment_type": "outbound", "status": "created", "status_label": "Created"}})
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "01234567890123456789012345678901", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := client.CreateShipment(context.Background(), CreateShipmentRequest{OrderID: "17", CustomerID: "5", Carrier: "NordShip", ServiceLevel: "standard", Recipient: Address{FirstName: "Ada"}, Items: []Item{{ProductID: "4", Name: "Book", Quantity: 1}}}, "shipment-order-17", "request-17")
	if err != nil {
		t.Fatal(err)
	}
	if shipment.ShipmentID != "shp_123" || attempts.Load() != 2 {
		t.Fatalf("shipment=%+v attempts=%d", shipment, attempts.Load())
	}
}

func TestCreateShipmentDoesNotRetryPermanentResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"success":false,"error":{"message":"bad token"}}`, want: ErrUnauthorized},
		{name: "conflict", status: http.StatusConflict, body: `{"success":false,"error":{"message":"duplicate"}}`, want: ErrConflict},
		{name: "server error", status: http.StatusInternalServerError, body: `{"success":false,"error":{"message":"failed"}}`, want: ErrUnavailable},
		{name: "malformed response", status: http.StatusOK, body: `{`, want: ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				attempts.Add(1)
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := NewHTTPClient(server.URL, "01234567890123456789012345678901", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.CreateShipment(context.Background(), CreateShipmentRequest{}, "shipment-order-17", "request-17")
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if attempts.Load() != 1 {
				t.Fatalf("permanent response was retried %d times", attempts.Load())
			}
		})
	}
}

func TestCreateShipmentMapsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(30 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "01234567890123456789012345678901", 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateShipment(context.Background(), CreateShipmentRequest{}, "shipment-order-17", "request-17")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestCreateReturnUsesVersionedEndpointAndIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/returns" || request.Header.Get("Idempotency-Key") != "return-order-17" {
			t.Fatalf("unexpected return request: %s key=%q", request.URL.Path, request.Header.Get("Idempotency-Key"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]any{"shipment_id": "shp_return_17", "tracking_number": "RET-DE-20260906-ABC123", "shipment_type": "return", "status": "return_requested", "status_label": "Return requested"}})
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "01234567890123456789012345678901", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := client.CreateReturn(context.Background(), CreateReturnRequest{OriginalShipmentID: "shp_17", OrderID: "17", CustomerID: "5", Sender: Address{FirstName: "Ada"}, Items: []Item{{ProductID: "4", Name: "Book", Quantity: 1}}}, "return-order-17", "request-17")
	if err != nil {
		t.Fatal(err)
	}
	if shipment.ShipmentID != "shp_return_17" || shipment.TrackingNumber != "RET-DE-20260906-ABC123" {
		t.Fatalf("unexpected return shipment: %+v", shipment)
	}
}

func TestTrackingURLUsesBrowserReachablePublicBaseURL(t *testing.T) {
	client, err := NewHTTPClient("http://shipping-app:8090", "01234567890123456789012345678901", time.Second, "https://tracking.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.TrackingURL("NS DE/17"); got != "https://tracking.example.test/track/NS%20DE%2F17" {
		t.Fatalf("unexpected tracking URL: %s", got)
	}
}

func TestGetTimelineUsesInternalAPIAndDecodesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/api/v1/shipments/NS-DE-17/events" {
			t.Fatalf("unexpected timeline path: %s", request.URL.EscapedPath())
		}
		if request.Header.Get("Authorization") != "Bearer 01234567890123456789012345678901" || request.Header.Get("X-Request-ID") != "request-17" {
			t.Fatal("timeline request did not propagate internal headers")
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]any{{"event_id": "evt_1", "status": "in_transit", "title": "In transit", "occurred_at": "2026-09-06T08:30:00Z"}}})
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "01234567890123456789012345678901", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.GetTimeline(context.Background(), "NS-DE-17", "request-17")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "evt_1" || events[0].Status != "in_transit" {
		t.Fatalf("unexpected timeline: %+v", events)
	}
}

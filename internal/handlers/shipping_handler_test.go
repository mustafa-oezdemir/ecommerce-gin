package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
)

type shippingHandlerServiceStub struct {
	callback    services.ShippingCallback
	callbackErr error
}

func (stub *shippingHandlerServiceStub) Enabled() bool { return true }
func (stub *shippingHandlerServiceStub) Handover(context.Context, uint, string) (*models.OrderShipment, error) {
	return nil, nil
}
func (stub *shippingHandlerServiceStub) ApplyCallback(_ context.Context, callback services.ShippingCallback) error {
	stub.callback = callback
	return stub.callbackErr
}

func TestShippingCallbackMapsSemanticValidationToBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &shippingHandlerServiceStub{callbackErr: services.ErrInvalidShippingCallback}
	handler := NewShippingHandler(stub, "current-token-012345678901234567890")
	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/api/v1/internal/shipping/events", handler.Callback)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/internal/shipping/events", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer current-token-012345678901234567890")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"INVALID_REQUEST"`) {
		t.Fatalf("expected structured 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestShippingCallbackAcceptsCurrentAndPreviousTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, token := range []string{"current-token-012345678901234567890", "previous-token-0123456789012345678"} {
		t.Run(token[:7], func(t *testing.T) {
			stub := &shippingHandlerServiceStub{}
			handler := NewShippingHandler(stub, "current-token-012345678901234567890", "previous-token-0123456789012345678")
			router := gin.New()
			router.Use(middleware.RequestID())
			router.POST("/api/v1/internal/shipping/events", handler.Callback)
			body := `{"event_id":"evt_1","shipment_id":"shp_1","order_id":"17","tracking_number":"NS-DE-17","shipment_type":"outbound","status":"in_transit","status_label":"In transit","occurred_at":"2026-09-06T08:30:00Z"}`
			request := httptest.NewRequest(http.MethodPost, "/api/v1/internal/shipping/events", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", response.Code, response.Body.String())
			}
			if stub.callback.EventID != "evt_1" || stub.callback.OrderID != "17" || stub.callback.OccurredAt.IsZero() {
				t.Fatalf("callback contract was not decoded: %+v", stub.callback)
			}
		})
	}
}

func TestShippingCallbackRejectsInvalidTokenAndUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &shippingHandlerServiceStub{}
	handler := NewShippingHandler(stub, "current-token-012345678901234567890")
	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/api/v1/internal/shipping/events", handler.Callback)

	invalidToken := httptest.NewRequest(http.MethodPost, "/api/v1/internal/shipping/events", strings.NewReader(`{}`))
	invalidToken.Header.Set("Authorization", "Bearer wrong")
	invalidTokenResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidTokenResponse, invalidToken)
	if invalidTokenResponse.Code != http.StatusUnauthorized || !strings.Contains(invalidTokenResponse.Body.String(), `"code":"UNAUTHORIZED"`) {
		t.Fatalf("expected structured 401, got %d: %s", invalidTokenResponse.Code, invalidTokenResponse.Body.String())
	}

	unknownField := httptest.NewRequest(http.MethodPost, "/api/v1/internal/shipping/events", strings.NewReader(`{"unexpected":true}`))
	unknownField.Header.Set("Authorization", "Bearer current-token-012345678901234567890")
	unknownFieldResponse := httptest.NewRecorder()
	router.ServeHTTP(unknownFieldResponse, unknownField)
	if unknownFieldResponse.Code != http.StatusBadRequest || !strings.Contains(unknownFieldResponse.Body.String(), `"code":"INVALID_REQUEST"`) {
		t.Fatalf("expected structured 400, got %d: %s", unknownFieldResponse.Code, unknownFieldResponse.Body.String())
	}
}

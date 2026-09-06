package handlers

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	appmetrics "github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	shippingapi "github.com/mustafa-oezdemir/ecommerce-gin/internal/shipping"
)

const maxShippingCallbackBytes int64 = 1 << 20

type ShippingHandler struct {
	service        shippingHandlerService
	callbackTokens []string
}

type shippingHandlerService interface {
	Enabled() bool
	Handover(context.Context, uint, string) (*models.OrderShipment, error)
	ApplyCallback(context.Context, services.ShippingCallback) error
}

func NewShippingHandler(service shippingHandlerService, callbackTokens ...string) *ShippingHandler {
	validTokens := make([]string, 0, len(callbackTokens))
	for _, token := range callbackTokens {
		if token = strings.TrimSpace(token); token != "" {
			validTokens = append(validTokens, token)
		}
	}
	return &ShippingHandler{service: service, callbackTokens: validTokens}
}

func (handler *ShippingHandler) Handover(c *gin.Context) {
	if !handler.service.Enabled() {
		c.String(http.StatusServiceUnavailable, "Shipping service is not configured")
		return
	}
	var uri struct {
		ID uint `uri:"id" binding:"required,gt=0"`
	}
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	shipment, err := handler.service.Handover(c.Request.Context(), uri.ID, c.GetString("request_id"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrShippingAddressAbsent), errors.Is(err, services.ErrShippingPaymentNotReady), errors.Is(err, services.ErrInvalidTransition):
			c.String(http.StatusConflict, "Order is not ready to hand over to shipping")
		case errors.Is(err, services.ErrShippingDisabled), errors.Is(err, shippingapi.ErrUnavailable):
			c.String(http.StatusServiceUnavailable, "Shipping service is temporarily unavailable")
		default:
			c.String(http.StatusInternalServerError, "Could not create shipment")
		}
		return
	}
	c.Redirect(http.StatusSeeOther, "/employee/orders?shipment="+shipment.TrackingNumber)
}

func (handler *ShippingHandler) Callback(c *gin.Context) {
	succeeded := false
	defer func() {
		if appMetrics := appmetrics.Default(); appMetrics != nil {
			appMetrics.ShippingCallbacks.Inc()
			if !succeeded {
				appMetrics.ShippingCallbackFailures.Inc()
			}
		}
	}()
	authorization := []byte(c.GetHeader("Authorization"))
	authenticated := 0
	for _, token := range handler.callbackTokens {
		authenticated |= subtle.ConstantTimeCompare(authorization, []byte("Bearer "+token))
	}
	if authenticated != 1 {
		shippingCallbackError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid service credentials are required")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxShippingCallbackBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		shippingCallbackError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Shipping callback exceeds 1 MiB")
		return
	}
	var callback services.ShippingCallback
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&callback); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		shippingCallbackError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid shipping callback")
		return
	}
	if err := handler.service.ApplyCallback(c.Request.Context(), callback); err != nil {
		if errors.Is(err, services.ErrInvalidShippingCallback) {
			shippingCallbackError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid shipping callback")
			return
		}
		shippingCallbackError(c, http.StatusInternalServerError, "CALLBACK_FAILED", "Could not record shipping callback")
		return
	}
	succeeded = true
	c.Status(http.StatusNoContent)
}

func shippingCallbackError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message, "request_id": c.GetString(middleware.RequestIDKey)},
	})
}

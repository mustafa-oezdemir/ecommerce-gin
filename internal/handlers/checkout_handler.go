package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

const maxWebhookBodyBytes int64 = 1 << 20

type CheckoutHandler struct {
	checkout  *services.CheckoutService
	addresses *services.AddressService
	orders    *services.OrderService
	webhooks  *services.PaymentWebhookService
	mailer    *services.MailService
}

func NewCheckoutHandler(database *gorm.DB, environment, webhookSecret string) *CheckoutHandler {
	gateway := services.NewReferencePaymentGateway(environment)
	return &CheckoutHandler{
		checkout:  services.NewCheckoutService(database, gateway),
		addresses: services.NewAddressService(database),
		orders:    services.NewOrderService(database),
		webhooks:  services.NewPaymentWebhookService(database, webhookSecret),
		mailer:    services.NewMailServiceFromEnv(),
	}
}

func (h *CheckoutHandler) Show(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	summary, err := h.checkout.Preview(c.Request.Context(), user.ID)
	if err != nil {
		if errors.Is(err, services.ErrCartEmpty) || errors.Is(err, services.ErrCartNotFound) {
			c.Redirect(http.StatusSeeOther, "/cart")
			return
		}
		c.String(http.StatusInternalServerError, "Could not load checkout")
		return
	}
	addresses, err := h.addresses.List(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load addresses")
		return
	}
	key, err := services.NewIdempotencyKey()
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not start checkout")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(http.StatusOK, "checkout/index", viewData(c, gin.H{"Summary": summary, "Addresses": addresses, "IdempotencyKey": key}))
}

func (h *CheckoutHandler) Place(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if metric := metrics.Default(); metric != nil {
		metric.CheckoutStarted.Inc()
	}
	var request validation.CheckoutRequest
	if err := c.ShouldBind(&request); err != nil {
		h.checkoutError(c, services.ErrInvalidCheckout)
		return
	}
	result, err := h.checkout.Checkout(c.Request.Context(), services.CheckoutInput{
		UserID: user.ID, ShippingAddressID: request.ShippingAddressID, BillingAddressID: request.BillingAddressID,
		BillingSameAsShipping: request.BillingSameAsShipping, PaymentMethod: models.PaymentMethod(request.PaymentMethod),
		IdempotencyKey: request.IdempotencyKey, ProviderToken: request.ProviderToken,
	})
	if err != nil {
		h.checkoutError(c, err)
		return
	}
	if !result.Replay {
		go h.mailer.SendOrderCreated(*user, *result.Order)
	}
	c.Redirect(http.StatusSeeOther, "/checkout/success?order_id="+strconv.FormatUint(uint64(result.Order.ID), 10))
}

func (h *CheckoutHandler) Success(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	orderID, err := strconv.ParseUint(c.Query("order_id"), 10, 64)
	if err != nil || orderID == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	order, err := h.orders.GetUserOrder(c.Request.Context(), user.ID, uint(orderID))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(http.StatusOK, "checkout/success", viewData(c, gin.H{"Order": order}))
}

func (h *CheckoutHandler) ListAddresses(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	addresses, err := h.addresses.List(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load addresses")
		return
	}
	c.HTML(http.StatusOK, "account/addresses/index", viewData(c, gin.H{"Addresses": addresses}))
}

func (h *CheckoutHandler) CreateAddress(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var request validation.AddressRequest
	if err := c.ShouldBind(&request); err != nil {
		c.String(http.StatusBadRequest, "Invalid address")
		return
	}
	if _, err := h.addresses.Create(c.Request.Context(), user.ID, addressFromRequest(request)); err != nil {
		c.String(http.StatusBadRequest, "Invalid address")
		return
	}
	c.Redirect(http.StatusSeeOther, safeAddressReturn(request.ReturnTo))
}

func (h *CheckoutHandler) UpdateAddress(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.AddressIDURI
	var request validation.AddressRequest
	if c.ShouldBindUri(&uri) != nil || c.ShouldBind(&request) != nil {
		c.String(http.StatusBadRequest, "Invalid address")
		return
	}
	if err := h.addresses.Update(c.Request.Context(), user.ID, uri.ID, addressFromRequest(request)); err != nil {
		if errors.Is(err, services.ErrAddressNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusBadRequest, "Invalid address")
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/addresses")
}

func (h *CheckoutHandler) DeleteAddress(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.AddressIDURI
	if c.ShouldBindUri(&uri) != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.addresses.Delete(c.Request.Context(), user.ID, uri.ID); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/addresses")
}

func (h *CheckoutHandler) Webhook(c *gin.Context) {
	provider := strings.TrimSpace(c.Param("provider"))
	if provider != "card" && provider != "paypal" && provider != "klarna" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
		return
	}
	if !h.webhooks.VerifySignature(body, c.GetHeader("X-Payment-Signature")) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var event services.PaymentWebhook
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&event) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.String(http.StatusBadRequest, "Invalid webhook")
		return
	}
	replay, err := h.webhooks.Process(c.Request.Context(), provider, event)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid webhook")
		return
	}
	if replay {
		slog.InfoContext(c.Request.Context(), "payment webhook replay ignored", "event", "webhook_replay", "provider", provider)
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *CheckoutHandler) checkoutError(c *gin.Context, err error) {
	metric := metrics.Default()
	reason := "internal_error"
	switch {
	case errors.Is(err, services.ErrCartNotFound), errors.Is(err, services.ErrCartEmpty), errors.Is(err, services.ErrInvalidQuantity), errors.Is(err, services.ErrInvalidCheckout):
		reason = "invalid_checkout"
		c.String(http.StatusBadRequest, "Checkout information is invalid or the cart is empty.")
	case errors.Is(err, services.ErrAddressNotFound):
		reason = "invalid_address"
		c.String(http.StatusBadRequest, "Select an address that belongs to your account.")
	case errors.Is(err, services.ErrInsufficientStock), errors.Is(err, services.ErrProductUnavailable):
		reason = "insufficient_stock"
		if metric != nil {
			metric.StockConflicts.Inc()
		}
		slog.InfoContext(c.Request.Context(), "checkout stock conflict", "event", "stock_conflict")
		c.String(http.StatusConflict, "One or more products are no longer available in the requested quantity.")
	case errors.Is(err, services.ErrIdempotencyConflict):
		reason = "idempotency_conflict"
		c.String(http.StatusConflict, "This checkout key was already used for another request.")
	case errors.Is(err, services.ErrPaymentFailed), errors.Is(err, services.ErrInvalidPayment):
		reason = "payment_failed"
		if metric != nil {
			metric.PaymentFailures.Inc()
		}
		slog.WarnContext(c.Request.Context(), "checkout payment failed", "event", "payment_failed")
		c.String(http.StatusBadGateway, "Payment could not be completed. Your cart was not changed.")
	default:
		slog.ErrorContext(c.Request.Context(), "checkout failed", "event", "checkout_failed", "error", err)
		c.String(http.StatusInternalServerError, "Checkout failed")
	}
	if metric != nil {
		metric.CheckoutFailures.WithLabelValues(reason).Inc()
	}
}

func addressFromRequest(request validation.AddressRequest) models.UserAddress {
	return models.UserAddress{FirstName: request.FirstName, LastName: request.LastName, Company: request.Company, Street: request.Street, HouseNumber: request.HouseNumber, AddressLine2: request.AddressLine2, PostalCode: request.PostalCode, City: request.City, State: request.State, CountryCode: request.CountryCode, Phone: request.Phone, IsDefault: request.IsDefault}
}

func safeAddressReturn(value string) string {
	if value == "/checkout" {
		return value
	}
	return "/account/addresses"
}

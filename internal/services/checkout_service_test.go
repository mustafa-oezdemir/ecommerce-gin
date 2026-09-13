package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/banking"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

func TestPaymentMethodsAreExplicitlyValidated(t *testing.T) {
	valid := []models.PaymentMethod{models.PaymentMethodDebitCard, models.PaymentMethodCreditCard, models.PaymentMethodPayPal, models.PaymentMethodKlarna, models.PaymentMethodPehlioneBank}
	for _, method := range valid {
		if !method.Valid() || method.Provider() == "" {
			t.Fatalf("payment method %q is not configured", method)
		}
	}
	if models.PaymentMethod("cash").Valid() || models.PaymentMethod("cash").Provider() != "" {
		t.Fatal("unknown payment method was accepted")
	}
}

type bankingClientStub struct {
	request banking.CreatePaymentIntentRequest
}

func (client *bankingClientStub) CreatePaymentIntent(_ context.Context, request banking.CreatePaymentIntentRequest) (*banking.PaymentIntent, error) {
	client.request = request
	return &banking.PaymentIntent{PaymentIntentID: "intent-123", Status: "AWAITING_CUSTOMER", ApprovalURL: "http://localhost:3000/pay/intent-123"}, nil
}

func TestRoutedGatewayCreatesPendingBankingIntent(t *testing.T) {
	client := &bankingClientStub{}
	gateway := NewRoutedPaymentGateway("test", client)
	result, err := gateway.CreatePayment(t.Context(), PaymentRequest{UserID: 7, Method: models.PaymentMethodPehlioneBank, AmountCents: 14999, Currency: "EUR", IdempotencyKey: strings.Repeat("a", 64), OrderReference: "ECOM-ORD-0000000123", RequestID: "request-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != models.PaymentStatusPending || result.PaymentIntentID != "intent-123" || result.ApprovalURL == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if client.request.Amount != "149.99" || client.request.OrderReference != "ECOM-ORD-0000000123" || client.request.RequestID != "request-1" {
		t.Fatalf("unexpected banking request: %#v", client.request)
	}
}

func TestReferencePaymentGatewayIsIdempotentAndProductionFailsClosed(t *testing.T) {
	request := PaymentRequest{UserID: 4, Method: models.PaymentMethodCreditCard, AmountCents: 2499, Currency: "EUR", IdempotencyKey: "0123456789abcdef0123456789abcdef"}
	gateway := NewReferencePaymentGateway("test")
	first, err := gateway.CreatePayment(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gateway.CreatePayment(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProviderPaymentID != second.ProviderPaymentID || first.Status != models.PaymentStatusPaid {
		t.Fatalf("reference payment retry was not idempotent: %#v %#v", first, second)
	}
	if _, err := NewReferencePaymentGateway("production").CreatePayment(t.Context(), request); !errors.Is(err, ErrPaymentFailed) {
		t.Fatalf("production reference gateway error = %v", err)
	}
}

func TestWebhookSignatureRequiresConfiguredSecretAndExactBody(t *testing.T) {
	service := NewPaymentWebhookService(nil, "0123456789abcdef0123456789abcdef")
	body := []byte("{\"event_id\":\"event_1\"}")
	mac := hmac.New(sha256.New, []byte("0123456789abcdef0123456789abcdef"))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	if !service.VerifySignature(body, signature) {
		t.Fatal("valid webhook signature rejected")
	}
	if service.VerifySignature(append(body, ' '), signature) {
		t.Fatal("modified webhook body accepted")
	}
	if NewPaymentWebhookService(nil, "").VerifySignature(body, signature) {
		t.Fatal("webhook accepted without configured secret")
	}
}

func TestAddressValidationNormalizesCountryAndRejectsIncompleteData(t *testing.T) {
	address := models.UserAddress{FirstName: " Ada ", LastName: " Lovelace ", Street: " Main ", HouseNumber: "1", PostalCode: "12345", City: "London", CountryCode: "gb", Phone: "+44 1"}
	if !validAddress(address) {
		t.Fatal("valid address rejected")
	}
	normalizeAddress(&address)
	if address.FirstName != "Ada" || address.CountryCode != "GB" {
		t.Fatalf("address was not normalized: %#v", address)
	}
	address.Street = ""
	if validAddress(address) {
		t.Fatal("incomplete address accepted")
	}
}

type retryGateway struct {
	mu         sync.Mutex
	calls      int
	logicalIDs map[string]string
}

func (gateway *retryGateway) CreatePayment(_ context.Context, request PaymentRequest) (*PaymentResult, error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.calls++
	if gateway.logicalIDs == nil {
		gateway.logicalIDs = make(map[string]string)
	}
	id := gateway.logicalIDs[request.IdempotencyKey]
	if id == "" {
		id = "retry_" + request.IdempotencyKey[:16]
		gateway.logicalIDs[request.IdempotencyKey] = id
		return nil, context.DeadlineExceeded
	}
	return &PaymentResult{ProviderPaymentID: id, Status: models.PaymentStatusPaid}, nil
}

func (gateway *retryGateway) ConfirmPayment(context.Context, string) (*PaymentResult, error) {
	return nil, ErrInvalidPayment
}
func (gateway *retryGateway) GetPaymentStatus(context.Context, string) (models.PaymentStatus, error) {
	return "", ErrInvalidPayment
}
func (gateway *retryGateway) RefundPayment(context.Context, string, int64) error { return nil }

type authorizedGateway struct{}

func (authorizedGateway) CreatePayment(_ context.Context, request PaymentRequest) (*PaymentResult, error) {
	return &PaymentResult{ProviderPaymentID: "authorized_" + request.IdempotencyKey[:16], Status: models.PaymentStatusAuthorized}, nil
}
func (authorizedGateway) ConfirmPayment(context.Context, string) (*PaymentResult, error) {
	return nil, ErrInvalidPayment
}
func (authorizedGateway) GetPaymentStatus(context.Context, string) (models.PaymentStatus, error) {
	return models.PaymentStatusAuthorized, nil
}
func (authorizedGateway) RefundPayment(context.Context, string, int64) error { return nil }

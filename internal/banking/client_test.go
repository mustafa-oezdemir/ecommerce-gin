package banking

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreatePaymentIntentSendsImmutableMerchantRequest(t *testing.T) {
	var server *httptest.Server
	expiresAt := time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/merchant/payment-intents" || request.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) {
			t.Fatalf("unexpected request path or authorization")
		}
		if request.Header.Get("Idempotency-Key") != "0123456789abcdef0123456789abcdef" || request.Header.Get("X-Request-ID") != "request-1" {
			t.Fatal("missing request metadata")
		}
		var body CreatePaymentIntentRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.MerchantID != "pehlione-ecommerce" || body.OrderReference != "ECOM-ORD-0000000123" || body.Amount != "149.99" || body.Currency != "EUR" {
			t.Fatalf("unexpected body: %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"payment_intent_id":"intent-1","merchant_id":"pehlione-ecommerce","merchant_name":"Pehlione E-Commerce GmbH","beneficiary_iban":"DE89999999998000000000","merchant_reference":"ECOM-ORD-0000000123","amount":"149.99","currency":"EUR","status":"AWAITING_CUSTOMER","return_url":"http://localhost:8080/checkout/banking/return","expires_at":"` + expiresAt + `","approval_url":"` + server.URL + `/pay/intent-1"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.URL, "pehlione-ecommerce", strings.Repeat("t", 32), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := client.CreatePaymentIntent(t.Context(), CreatePaymentIntentRequest{OrderReference: "ECOM-ORD-0000000123", Amount: "149.99", Currency: "EUR", IdempotencyKey: "0123456789abcdef0123456789abcdef", RequestID: "request-1"})
	if err != nil || intent.PaymentIntentID != "intent-1" || intent.Amount != "149.99" || intent.MerchantReference != "ECOM-ORD-0000000123" {
		t.Fatalf("intent=%#v err=%v", intent, err)
	}
}

func TestCreatePaymentIntentRejectsMismatchedImmutableEcho(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"payment_intent_id":"intent-1","merchant_id":"pehlione-ecommerce","merchant_name":"Pehlione E-Commerce GmbH","beneficiary_iban":"DE89999999998000000000","merchant_reference":"ECOM-ORD-OTHER","amount":"149.99","currency":"EUR","status":"AWAITING_CUSTOMER","return_url":"http://localhost:8080/checkout/banking/return","expires_at":"` + time.Now().UTC().Add(15*time.Minute).Format(time.RFC3339) + `","approval_url":"` + server.URL + `/pay/intent-1"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.URL, "pehlione-ecommerce", strings.Repeat("t", 32), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreatePaymentIntent(t.Context(), CreatePaymentIntentRequest{OrderReference: "ECOM-ORD-0000000123", Amount: "149.99", Currency: "EUR", IdempotencyKey: strings.Repeat("a", 32)})
	if err == nil {
		t.Fatal("mismatched merchant reference was accepted")
	}
}

func TestApproveCardPaymentAcceptsBankingResponseContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/merchant/payment-intents/intent-1/card-approve" {
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"payment_intent_id":"intent-1","merchant_id":"pehlione-ecommerce","merchant_name":"Pehlione E-Commerce GmbH","beneficiary_iban":"DE89999999998000000000","merchant_reference":"ECOM-ORD-0000000123","amount":"149.99","currency":"EUR","status":"BOOKED","payment_id":"payment-1","return_url":"http://localhost:8080/checkout/banking/return","expires_at":"2027-01-01T00:00:00Z","brand":"visa","last4":"8831"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.URL, "pehlione-ecommerce", strings.Repeat("t", 32), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payment, err := client.ApproveCardPayment(t.Context(), ApproveCardPaymentRequest{PaymentIntentID: "intent-1", CardToken: strings.Repeat("a", 64), CVC: "123"})
	if err != nil || payment.PaymentID != "payment-1" || payment.Status != "BOOKED" {
		t.Fatalf("payment=%#v err=%v", payment, err)
	}
}

func TestCreatePaymentIntentRejectsForeignApprovalURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"payment_intent_id":"intent-1","status":"AWAITING_CUSTOMER","approval_url":"https://evil.example/pay/intent-1"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "http://localhost:3000", "pehlione-ecommerce", strings.Repeat("t", 32), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreatePaymentIntent(t.Context(), CreatePaymentIntentRequest{OrderReference: "ECOM-ORD-1", Amount: "1.00", Currency: "EUR", IdempotencyKey: "0123456789abcdef0123456789abcdef"})
	if err == nil {
		t.Fatal("foreign approval URL accepted")
	}
}

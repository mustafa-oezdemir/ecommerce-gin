package banking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes int64 = 1 << 20

var ErrInvalidPaymentIntent = errors.New("invalid banking payment intent")

type Client struct {
	apiURL       *url.URL
	publicOrigin *url.URL
	merchantID   string
	token        string
	httpClient   *http.Client
}

func NewClient(apiURL, publicURL, merchantID, token string, timeout time.Duration) (*Client, error) {
	apiOrigin, err := parseOrigin("BANKING_API_URL", apiURL, false)
	if err != nil {
		return nil, err
	}
	publicOrigin, err := parseOrigin("BANKING_PUBLIC_URL", publicURL, false)
	if err != nil {
		return nil, err
	}
	merchantID, token = strings.TrimSpace(merchantID), strings.TrimSpace(token)
	if merchantID == "" || len(merchantID) > 64 || len(token) < 32 || timeout <= 0 {
		return nil, errors.New("banking client requires merchant id, 32-character token, and positive timeout")
	}
	return &Client{apiURL: apiOrigin, publicOrigin: publicOrigin, merchantID: merchantID, token: token, httpClient: &http.Client{Timeout: timeout}}, nil
}

func (client *Client) CreatePaymentIntent(ctx context.Context, request CreatePaymentIntentRequest) (*PaymentIntent, error) {
	request.MerchantID = client.merchantID
	if request.OrderReference == "" || request.Amount == "" || request.Currency != "EUR" || len(request.IdempotencyKey) < 32 {
		return nil, ErrInvalidPaymentIntent
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode banking payment intent: %w", err)
	}
	endpoint := client.apiURL.ResolveReference(&url.URL{Path: "/api/v1/merchant/payment-intents"})
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create banking request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.token)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Idempotency-Key", request.IdempotencyKey)
	if request.RequestID != "" {
		httpRequest.Header.Set("X-Request-ID", request.RequestID)
	}
	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("create banking payment intent: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read banking response: %w", err)
	}
	if int64(len(responseBody)) > maxResponseBytes || response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create banking payment intent: status %d", response.StatusCode)
	}
	var intent PaymentIntent
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&intent) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, ErrInvalidPaymentIntent
	}
	approval, err := url.Parse(intent.ApprovalURL)
	if err != nil || approval.Scheme != client.publicOrigin.Scheme || approval.Host != client.publicOrigin.Host || !strings.HasPrefix(approval.EscapedPath(), "/pay/") || approval.RawQuery != "" || approval.Fragment != "" {
		return nil, ErrInvalidPaymentIntent
	}
	if strings.TrimSpace(intent.PaymentIntentID) == "" || len(intent.PaymentIntentID) > 128 || intent.Status != "AWAITING_CUSTOMER" {
		return nil, ErrInvalidPaymentIntent
	}
	return &intent, nil
}

func (client *Client) TokenizeCard(ctx context.Context, request TokenizeCardRequest) (*TokenizedCard, error) {
	if len(strings.TrimSpace(request.CardNumber)) < 13 || request.ExpMonth < 1 || request.ExpMonth > 12 || request.ExpYear < time.Now().UTC().Year() || len(strings.TrimSpace(request.CVC)) != 3 {
		return nil, ErrInvalidPaymentIntent
	}
	payload := struct {
		MerchantID string `json:"merchant_id"`
		CardNumber string `json:"card_number"`
		ExpMonth   int    `json:"exp_month"`
		ExpYear    int    `json:"exp_year"`
		CVC        string `json:"cvc"`
	}{client.merchantID, request.CardNumber, request.ExpMonth, request.ExpYear, request.CVC}
	var result TokenizedCard
	if err := client.post(ctx, "/api/v1/merchant/cards/tokenize", request.RequestID, payload, &result, http.StatusCreated); err != nil {
		return nil, err
	}
	if len(result.Token) != 64 || len(result.Last4) != 4 || result.Brand == "" || result.ExpMonth < 1 || result.ExpMonth > 12 {
		return nil, ErrInvalidPaymentIntent
	}
	return &result, nil
}

func (client *Client) ApproveCardPayment(ctx context.Context, request ApproveCardPaymentRequest) (*CardPayment, error) {
	if strings.TrimSpace(request.PaymentIntentID) == "" || len(request.CardToken) != 64 || len(strings.TrimSpace(request.CVC)) != 3 {
		return nil, ErrInvalidPaymentIntent
	}
	payload := struct {
		MerchantID string `json:"merchant_id"`
		CardToken  string `json:"card_token"`
		CVC        string `json:"cvc"`
	}{client.merchantID, request.CardToken, request.CVC}
	var result CardPayment
	path := "/api/v1/merchant/payment-intents/" + url.PathEscape(request.PaymentIntentID) + "/card-approve"
	if err := client.post(ctx, path, request.RequestID, payload, &result, http.StatusOK); err != nil {
		return nil, err
	}
	if result.PaymentIntentID != request.PaymentIntentID || result.Status != "BOOKED" || result.PaymentID == "" || len(result.Last4) != 4 || result.Brand == "" {
		return nil, ErrInvalidPaymentIntent
	}
	return &result, nil
}

func (client *Client) post(ctx context.Context, path, requestID string, payload, destination any, expectedStatus int) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode banking request: %w", err)
	}
	endpoint := client.apiURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create banking request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send banking request: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read banking response: %w", err)
	}
	if int64(len(responseBody)) > maxResponseBytes || response.StatusCode != expectedStatus {
		return fmt.Errorf("banking request: status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return ErrInvalidPaymentIntent
	}
	return nil
}

func parseOrigin(name, raw string, requireHTTPS bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" || parsed.Scheme != "http" && parsed.Scheme != "https" || requireHTTPS && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s must be an absolute http(s) origin", name)
	}
	parsed.Path = ""
	return parsed, nil
}

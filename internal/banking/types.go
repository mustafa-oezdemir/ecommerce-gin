package banking

import "context"

type CreatePaymentIntentRequest struct {
	MerchantID     string `json:"merchant_id"`
	OrderReference string `json:"order_reference"`
	Amount         string `json:"amount"`
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"-"`
	RequestID      string `json:"-"`
}

type PaymentIntent struct {
	PaymentIntentID string `json:"payment_intent_id"`
	Status          string `json:"status"`
	ApprovalURL     string `json:"approval_url"`
}

type PaymentIntentClient interface {
	CreatePaymentIntent(context.Context, CreatePaymentIntentRequest) (*PaymentIntent, error)
}

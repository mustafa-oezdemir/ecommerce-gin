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

type TokenizeCardRequest struct {
	CardNumber string `json:"card_number"`
	ExpMonth   int    `json:"exp_month"`
	ExpYear    int    `json:"exp_year"`
	CVC        string `json:"cvc"`
	RequestID  string `json:"-"`
}

type TokenizedCard struct {
	Token    string `json:"token"`
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

type ApproveCardPaymentRequest struct {
	PaymentIntentID string `json:"-"`
	CardToken       string `json:"card_token"`
	CVC             string `json:"cvc"`
	RequestID       string `json:"-"`
}

type CardPayment struct {
	PaymentIntentID string `json:"payment_intent_id"`
	Status          string `json:"status"`
	PaymentID       string `json:"payment_id"`
	Brand           string `json:"brand"`
	Last4           string `json:"last4"`
}

type CardClient interface {
	TokenizeCard(context.Context, TokenizeCardRequest) (*TokenizedCard, error)
	ApproveCardPayment(context.Context, ApproveCardPaymentRequest) (*CardPayment, error)
}

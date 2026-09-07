package services

import (
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

func TestOrderStatusTransitions(t *testing.T) {
	valid := []struct{ from, to models.OrderStatus }{{models.OrderStatusPendingPayment, models.OrderStatusPaid}, {models.OrderStatusPaid, models.OrderStatusPreparing}, {models.OrderStatusPreparing, models.OrderStatusReadyForShipping}, {models.OrderStatusPending, models.OrderStatusPreparing}}
	for _, tt := range valid {
		if !CanTransitionOrderStatus(tt.from, tt.to) {
			t.Fatalf("expected %s -> %s to be valid", tt.from, tt.to)
		}
	}
	if CanTransitionOrderStatus(models.OrderStatusCompleted, models.OrderStatusPending) {
		t.Fatal("completed order cannot become pending")
	}
	invalid := []struct{ from, to models.OrderStatus }{{models.OrderStatusPending, models.OrderStatusShipped}, {models.OrderStatusPreparing, models.OrderStatusPreparing}, {models.OrderStatusReadyForShipping, models.OrderStatusShipped}, {models.OrderStatusShipped, models.OrderStatusCompleted}, {models.OrderStatusCancelled, models.OrderStatusPreparing}, {models.OrderStatusPending, models.OrderStatusCancelled}, {models.OrderStatusPaid, models.OrderStatusCancelled}, {models.OrderStatusReadyForShipping, models.OrderStatusCancelled}}
	for _, tt := range invalid {
		if CanTransitionOrderStatus(tt.from, tt.to) {
			t.Fatalf("expected %s -> %s to be invalid", tt.from, tt.to)
		}
	}
}

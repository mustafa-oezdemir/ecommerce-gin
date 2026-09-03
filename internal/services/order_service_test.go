package services

import (
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"testing"
)

func TestOrderStatusTransitions(t *testing.T) {
	valid := []struct{ from, to models.OrderStatus }{{models.OrderStatusPending, models.OrderStatusProcessing}, {models.OrderStatusPending, models.OrderStatusCancelled}, {models.OrderStatusProcessing, models.OrderStatusShipped}, {models.OrderStatusProcessing, models.OrderStatusCancelled}, {models.OrderStatusShipped, models.OrderStatusCompleted}}
	for _, tt := range valid {
		if !CanTransitionOrderStatus(tt.from, tt.to) {
			t.Fatalf("expected %s -> %s to be valid", tt.from, tt.to)
		}
	}
	if CanTransitionOrderStatus(models.OrderStatusCompleted, models.OrderStatusPending) {
		t.Fatal("completed order cannot become pending")
	}
	invalid := []struct{ from, to models.OrderStatus }{{models.OrderStatusPending, models.OrderStatusShipped}, {models.OrderStatusProcessing, models.OrderStatusProcessing}, {models.OrderStatusShipped, models.OrderStatusCancelled}, {models.OrderStatusCancelled, models.OrderStatusProcessing}}
	for _, tt := range invalid {
		if CanTransitionOrderStatus(tt.from, tt.to) {
			t.Fatalf("expected %s -> %s to be invalid", tt.from, tt.to)
		}
	}
}

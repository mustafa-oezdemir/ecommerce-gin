package services

import (
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"testing"
)

func TestOrderStatusTransitions(t *testing.T) {
	valid := []struct{ from, to models.OrderStatus }{{models.OrderStatusPending, models.OrderStatusProcessing}, {models.OrderStatusPending, models.OrderStatusCancelled}, {models.OrderStatusProcessing, models.OrderStatusShipped}, {models.OrderStatusShipped, models.OrderStatusCompleted}}
	for _, tt := range valid {
		if !CanTransitionOrderStatus(tt.from, tt.to) {
			t.Fatalf("expected %s -> %s to be valid", tt.from, tt.to)
		}
	}
	if CanTransitionOrderStatus(models.OrderStatusCompleted, models.OrderStatusPending) {
		t.Fatal("completed order cannot become pending")
	}
}

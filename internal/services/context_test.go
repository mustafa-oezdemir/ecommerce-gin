package services

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestServicesPropagateCancelledContext(t *testing.T) {
	database, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "user:password@tcp(127.0.0.1:1)/unreachable",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger: logger.New(log.New(io.Discard, "", 0), logger.Config{
			LogLevel: logger.Silent,
		}),
	})
	if err != nil {
		t.Fatalf("create disconnected test database: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "cart", run: func() error {
			_, err := NewCartService(database).GetCart(ctx, models.User{Model: gorm.Model{ID: 1}})
			return err
		}},
		{name: "orders", run: func() error {
			_, err := NewOrderService(database).ListUserOrders(ctx, 1)
			return err
		}},
		{name: "product lists", run: func() error {
			_, err := NewProductListService(database).List(ctx, 1)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context cancellation, got %v", err)
			}
		})
	}
}

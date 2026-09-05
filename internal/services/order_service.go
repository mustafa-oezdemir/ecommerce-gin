package services

import (
	"context"
	"errors"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidUser        = errors.New("invalid user")
	ErrCartNotFound       = errors.New("cart not found")
	ErrCartEmpty          = errors.New("cart is empty")
	ErrInvalidQuantity    = errors.New("invalid quantity")
	ErrProductUnavailable = errors.New("product unavailable")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrInvalidTransition  = errors.New("invalid order status transition")
)

type OrderService struct {
	database  *gorm.DB
	orderRepo orderRepository
}

type orderRepository interface {
	ListByUserID(ctx context.Context, userID uint) ([]models.Order, error)
	GetByIDForUser(ctx context.Context, orderID, userID uint) (*models.Order, error)
}

func NewOrderService(database *gorm.DB) *OrderService {
	return newOrderService(database, repositories.NewOrderRepository(database))
}

func newOrderService(database *gorm.DB, repo orderRepository) *OrderService {
	return &OrderService{database: database, orderRepo: repo}
}

func (s *OrderService) ListUserOrders(ctx context.Context, userID uint) ([]models.Order, error) {
	if userID == 0 {
		return nil, ErrInvalidUser
	}
	return s.orderRepo.ListByUserID(ctx, userID)
}

func (s *OrderService) GetUserOrder(ctx context.Context, userID, orderID uint) (*models.Order, error) {
	if userID == 0 || orderID == 0 {
		return nil, ErrInvalidUser
	}
	return s.orderRepo.GetByIDForUser(ctx, orderID, userID)
}

func CanTransitionOrderStatus(from, to models.OrderStatus) bool {
	return models.CanTransitionOrderStatus(from, to)
}

func (s *OrderService) UpdateStatus(ctx context.Context, orderID uint, status models.OrderStatus) error {
	if orderID == 0 {
		return ErrInvalidTransition
	}
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
			return err
		}
		if !CanTransitionOrderStatus(order.Status, status) {
			return ErrInvalidTransition
		}
		return tx.Model(&order).Update("status", status).Error
	})
}

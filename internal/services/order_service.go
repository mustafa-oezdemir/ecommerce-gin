package services

import (
	"context"
	"errors"
	"fmt"
	"sort"

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

func (s *OrderService) CreateOrder(ctx context.Context, user models.User) (*models.Order, error) {
	if user.ID == 0 {
		return nil, ErrInvalidUser
	}
	var created models.Order
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cart models.Cart
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", user.ID).First(&cart).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCartNotFound
			}
			return fmt.Errorf("load cart: %w", err)
		}
		var cartItems []models.CartItem
		if err := tx.Where("cart_id = ?", cart.ID).Find(&cartItems).Error; err != nil {
			return fmt.Errorf("load cart items: %w", err)
		}
		if len(cartItems) == 0 {
			return ErrCartEmpty
		}
		sort.Slice(cartItems, func(i, j int) bool { return cartItems[i].ProductID < cartItems[j].ProductID })

		items := make([]models.OrderItem, 0, len(cartItems))
		var totalCents int64
		for _, cartItem := range cartItems {
			if cartItem.Quantity < 1 || cartItem.Quantity > 100 {
				return ErrInvalidQuantity
			}
			var product models.Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, cartItem.ProductID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrProductUnavailable
				}
				return fmt.Errorf("load product: %w", err)
			}
			if !product.Active {
				return ErrProductUnavailable
			}
			if product.Stock < cartItem.Quantity {
				return ErrInsufficientStock
			}
			subtotal := product.PriceCents * int64(cartItem.Quantity)
			if subtotal <= 0 || totalCents > (1<<63-1)-subtotal {
				return fmt.Errorf("invalid order total")
			}
			totalCents += subtotal
			items = append(items, models.OrderItem{ProductID: product.ID, ProductName: product.Name, UnitPriceCents: product.PriceCents, Quantity: cartItem.Quantity, SubtotalCents: subtotal})
		}

		order := models.Order{UserID: user.ID, Status: models.OrderStatusPending, TotalCents: totalCents}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("create order: %w", err)
		}
		for i := range items {
			items[i].OrderID = order.ID
			if err := tx.Create(&items[i]).Error; err != nil {
				return fmt.Errorf("create order item: %w", err)
			}
			result := tx.Model(&models.Product{}).Where("id = ? AND stock >= ?", items[i].ProductID, items[i].Quantity).Update("stock", gorm.Expr("stock - ?", items[i].Quantity))
			if result.Error != nil {
				return fmt.Errorf("decrement stock: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrInsufficientStock
			}
		}
		if err := tx.Where("cart_id = ?", cart.ID).Delete(&models.CartItem{}).Error; err != nil {
			return fmt.Errorf("clear cart: %w", err)
		}
		order.Items = items
		created = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
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

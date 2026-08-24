package services

import (
	"errors"
	"fmt"
	"sort"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
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

type OrderService struct{ orderRepo *repositories.OrderRepository }

func NewOrderService() *OrderService {
	return &OrderService{orderRepo: repositories.NewOrderRepository()}
}

func (s *OrderService) CreateOrder(user models.User) (*models.Order, error) {
	if user.ID == 0 {
		return nil, ErrInvalidUser
	}
	var created models.Order
	err := db.DB.Transaction(func(tx *gorm.DB) error {
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

func (s *OrderService) ListUserOrders(userID uint) ([]models.Order, error) {
	if userID == 0 {
		return nil, ErrInvalidUser
	}
	return s.orderRepo.ListByUserID(userID)
}

func (s *OrderService) GetUserOrder(userID, orderID uint) (*models.Order, error) {
	if userID == 0 || orderID == 0 {
		return nil, ErrInvalidUser
	}
	return s.orderRepo.GetByIDForUser(orderID, userID)
}

func CanTransitionOrderStatus(from, to models.OrderStatus) bool {
	switch from {
	case models.OrderStatusPending:
		return to == models.OrderStatusProcessing || to == models.OrderStatusCancelled
	case models.OrderStatusProcessing:
		return to == models.OrderStatusShipped || to == models.OrderStatusCancelled
	case models.OrderStatusShipped:
		return to == models.OrderStatusCompleted
	default:
		return false
	}
}

func (s *OrderService) UpdateStatus(orderID uint, status models.OrderStatus) error {
	if orderID == 0 {
		return ErrInvalidTransition
	}
	return db.DB.Transaction(func(tx *gorm.DB) error {
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

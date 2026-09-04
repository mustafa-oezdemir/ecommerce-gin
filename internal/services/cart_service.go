package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"gorm.io/gorm"
)

var (
	ErrInvalidCartInput = errors.New("invalid cart input")
	ErrProductNotFound  = errors.New("product not found")
	ErrProductInactive  = errors.New("product inactive")
	ErrCartItemNotFound = errors.New("cart item not found")
)

type CartService struct {
	database *gorm.DB
	repo     cartRepository
}

type cartRepository interface {
	GetOrCreateCart(ctx context.Context, userID uint) (*models.Cart, error)
	AddItem(ctx context.Context, cartID, productID uint, qty int) error
	ClearCart(ctx context.Context, cartID uint) error
	UpdateQuantityForUser(ctx context.Context, userID, itemID uint, quantity int) (bool, error)
	RemoveItemForUser(ctx context.Context, userID, itemID uint) (bool, error)
}

func NewCartService(database *gorm.DB) *CartService {
	return newCartService(database, repositories.NewCartRepository(database))
}

func newCartService(database *gorm.DB, repo cartRepository) *CartService {
	return &CartService{database: database, repo: repo}
}

func (s *CartService) AddToCart(ctx context.Context, user models.User, productID uint, qty int) error {
	if user.ID == 0 || productID == 0 || qty < 1 || qty > 100 {
		return ErrInvalidCartInput
	}
	var product models.Product
	if err := s.database.WithContext(ctx).First(&product, productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductNotFound
		}
		return fmt.Errorf("load product: %w", err)
	}
	if !product.Active {
		return ErrProductInactive
	}
	cart, err := s.repo.GetOrCreateCart(ctx, user.ID)
	if err != nil {
		return err
	}
	return s.repo.AddItem(ctx, cart.ID, productID, qty)
}

func (s *CartService) GetCart(ctx context.Context, user models.User) (*models.Cart, error) {
	if user.ID == 0 {
		return nil, ErrInvalidCartInput
	}
	return s.repo.GetOrCreateCart(ctx, user.ID)
}

func (s *CartService) UpdateQuantity(ctx context.Context, user models.User, itemID uint, quantity int) error {
	if user.ID == 0 || itemID == 0 || quantity < 1 || quantity > 100 {
		return ErrInvalidCartInput
	}
	found, err := s.repo.UpdateQuantityForUser(ctx, user.ID, itemID, quantity)
	if err != nil {
		return err
	}
	if !found {
		return ErrCartItemNotFound
	}
	return nil
}

func (s *CartService) RemoveItem(ctx context.Context, user models.User, itemID uint) error {
	if user.ID == 0 || itemID == 0 {
		return ErrInvalidCartInput
	}
	found, err := s.repo.RemoveItemForUser(ctx, user.ID, itemID)
	if err != nil {
		return err
	}
	if !found {
		return ErrCartItemNotFound
	}
	return nil
}

func (s *CartService) ClearCart(ctx context.Context, user models.User) error {
	cart, err := s.repo.GetOrCreateCart(ctx, user.ID)
	if err != nil {
		return err
	}
	return s.repo.ClearCart(ctx, cart.ID)
}

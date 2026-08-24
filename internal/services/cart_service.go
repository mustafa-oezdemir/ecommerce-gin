package services

import (
	"errors"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
)

var (
	ErrInvalidCartInput = errors.New("invalid cart input")
	ErrProductNotFound  = errors.New("product not found")
	ErrProductInactive  = errors.New("product inactive")
	ErrCartItemNotFound = errors.New("cart item not found")
)

type CartService struct {
	repo *repositories.CartRepository
}

func NewCartService() *CartService {
	return &CartService{
		repo: repositories.NewCartRepository(),
	}
}

func (s *CartService) AddToCart(user models.User, productID uint, qty int) error {
	if user.ID == 0 || productID == 0 || qty < 1 || qty > 100 {
		return ErrInvalidCartInput
	}
	var product models.Product
	if err := db.DB.First(&product, productID).Error; err != nil {
		return ErrProductNotFound
	}
	if !product.Active {
		return ErrProductInactive
	}
	cart, err := s.repo.GetOrCreateCart(user.ID)
	if err != nil {
		return err
	}
	return s.repo.AddItem(cart.ID, productID, qty)
}

func (s *CartService) GetCart(user models.User) (*models.Cart, error) {
	if user.ID == 0 {
		return nil, ErrInvalidCartInput
	}
	return s.repo.GetOrCreateCart(user.ID)
}

func (s *CartService) UpdateQuantity(user models.User, itemID uint, quantity int) error {
	if user.ID == 0 || itemID == 0 || quantity < 1 || quantity > 100 {
		return ErrInvalidCartInput
	}
	found, err := s.repo.UpdateQuantityForUser(user.ID, itemID, quantity)
	if err != nil {
		return err
	}
	if !found {
		return ErrCartItemNotFound
	}
	return nil
}

func (s *CartService) RemoveItem(user models.User, itemID uint) error {
	if user.ID == 0 || itemID == 0 {
		return ErrInvalidCartInput
	}
	found, err := s.repo.RemoveItemForUser(user.ID, itemID)
	if err != nil {
		return err
	}
	if !found {
		return ErrCartItemNotFound
	}
	return nil
}

func (s *CartService) ClearCart(user models.User) error {
	cart, err := s.repo.GetOrCreateCart(user.ID)
	if err != nil {
		return err
	}
	return s.repo.ClearCart(cart.ID)
}

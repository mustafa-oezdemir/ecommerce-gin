package services

import (
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
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
	cart, err := s.repo.GetOrCreateCart(user.ID)
	if err != nil {
		return err
	}
	return s.repo.AddItem(cart.ID, productID, qty)
}

func (s *CartService) GetCart(user models.User) (*models.Cart, error) {
	return s.repo.GetOrCreateCart(user.ID)
}

func (s *CartService) ClearCart(user models.User) error {
	cart, err := s.repo.GetOrCreateCart(user.ID)
	if err != nil {
		return err
	}
	return s.repo.ClearCart(cart.ID)
}

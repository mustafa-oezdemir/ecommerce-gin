package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"gorm.io/gorm"
)

var (
	ErrInvalidProductListInput = errors.New("invalid product list input")
	ErrProductListNotFound     = errors.New("product list not found")
)

type ProductListService struct {
	database *gorm.DB
	repo     productListRepository
}

type productListRepository interface {
	ListByUserID(ctx context.Context, userID uint) ([]models.ProductList, error)
	GetByIDForUser(ctx context.Context, listID, userID uint) (*models.ProductList, error)
	Create(ctx context.Context, list *models.ProductList) error
	AddProduct(ctx context.Context, listID, productID uint) error
	RemoveProduct(ctx context.Context, listID, userID, productID uint) (bool, error)
	Delete(ctx context.Context, listID, userID uint) (bool, error)
}

func NewProductListService(database *gorm.DB) *ProductListService {
	return newProductListService(database, repositories.NewProductListRepository(database))
}

func newProductListService(database *gorm.DB, repo productListRepository) *ProductListService {
	return &ProductListService{database: database, repo: repo}
}

func (s *ProductListService) List(ctx context.Context, userID uint) ([]models.ProductList, error) {
	if userID == 0 {
		return nil, ErrInvalidProductListInput
	}
	return s.repo.ListByUserID(ctx, userID)
}

func (s *ProductListService) Get(ctx context.Context, userID, listID uint) (*models.ProductList, error) {
	if userID == 0 || listID == 0 {
		return nil, ErrInvalidProductListInput
	}
	list, err := s.repo.GetByIDForUser(ctx, listID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrProductListNotFound
	}
	return list, err
}

func (s *ProductListService) Create(ctx context.Context, userID uint, name string) error {
	name = strings.TrimSpace(name)
	if userID == 0 || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return ErrInvalidProductListInput
	}
	return s.repo.Create(ctx, &models.ProductList{UserID: userID, Name: name})
}

func (s *ProductListService) AddProduct(ctx context.Context, userID, listID, productID uint) error {
	if userID == 0 || listID == 0 || productID == 0 {
		return ErrInvalidProductListInput
	}
	if _, err := s.Get(ctx, userID, listID); err != nil {
		return err
	}
	var product models.Product
	if err := s.database.WithContext(ctx).Where("id = ? AND active = ?", productID, true).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductNotFound
		}
		return fmt.Errorf("load product: %w", err)
	}
	return s.repo.AddProduct(ctx, listID, productID)
}

func (s *ProductListService) RemoveProduct(ctx context.Context, userID, listID, productID uint) error {
	if userID == 0 || listID == 0 || productID == 0 {
		return ErrInvalidProductListInput
	}
	removed, err := s.repo.RemoveProduct(ctx, listID, userID, productID)
	if err != nil {
		return err
	}
	if !removed {
		return ErrProductListNotFound
	}
	return nil
}

func (s *ProductListService) Delete(ctx context.Context, userID, listID uint) error {
	if userID == 0 || listID == 0 {
		return ErrInvalidProductListInput
	}
	deleted, err := s.repo.Delete(ctx, listID, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrProductListNotFound
	}
	return nil
}

func (s *ProductListService) AvailableProducts(ctx context.Context) ([]models.Product, error) {
	var products []models.Product
	err := s.database.WithContext(ctx).Preload("Category").Where("active = ?", true).Order("name ASC").Find(&products).Error
	return products, err
}

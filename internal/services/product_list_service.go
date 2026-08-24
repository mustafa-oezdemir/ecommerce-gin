package services

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"gorm.io/gorm"
)

var (
	ErrInvalidProductListInput = errors.New("invalid product list input")
	ErrProductListNotFound     = errors.New("product list not found")
)

type ProductListService struct {
	repo *repositories.ProductListRepository
}

func NewProductListService() *ProductListService {
	return &ProductListService{repo: repositories.NewProductListRepository()}
}

func (s *ProductListService) List(userID uint) ([]models.ProductList, error) {
	if userID == 0 {
		return nil, ErrInvalidProductListInput
	}
	return s.repo.ListByUserID(userID)
}

func (s *ProductListService) Get(userID, listID uint) (*models.ProductList, error) {
	if userID == 0 || listID == 0 {
		return nil, ErrInvalidProductListInput
	}
	list, err := s.repo.GetByIDForUser(listID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrProductListNotFound
	}
	return list, err
}

func (s *ProductListService) Create(userID uint, name string) error {
	name = strings.TrimSpace(name)
	if userID == 0 || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return ErrInvalidProductListInput
	}
	return s.repo.Create(&models.ProductList{UserID: userID, Name: name})
}

func (s *ProductListService) AddProduct(userID, listID, productID uint) error {
	if userID == 0 || listID == 0 || productID == 0 {
		return ErrInvalidProductListInput
	}
	if _, err := s.Get(userID, listID); err != nil {
		return err
	}
	var product models.Product
	if err := db.DB.Where("id = ? AND active = ?", productID, true).First(&product).Error; err != nil {
		return ErrProductNotFound
	}
	return s.repo.AddProduct(listID, productID)
}

func (s *ProductListService) RemoveProduct(userID, listID, productID uint) error {
	if userID == 0 || listID == 0 || productID == 0 {
		return ErrInvalidProductListInput
	}
	removed, err := s.repo.RemoveProduct(listID, userID, productID)
	if err != nil {
		return err
	}
	if !removed {
		return ErrProductListNotFound
	}
	return nil
}

func (s *ProductListService) Delete(userID, listID uint) error {
	if userID == 0 || listID == 0 {
		return ErrInvalidProductListInput
	}
	deleted, err := s.repo.Delete(listID, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrProductListNotFound
	}
	return nil
}

func (s *ProductListService) AvailableProducts() ([]models.Product, error) {
	var products []models.Product
	err := db.DB.Preload("Category").Where("active = ?", true).Order("name ASC").Find(&products).Error
	return products, err
}

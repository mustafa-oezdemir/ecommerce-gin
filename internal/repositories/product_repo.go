package repositories

import (
	"context"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type ProductQuery struct {
	Limit         int
	Offset        int
	CategoryID    uint
	MinPriceCents *int64
	MaxPriceCents *int64
	Search        string
	Sort          string
	Order         string
}

type ProductRepository struct {
	database *gorm.DB
}

func NewProductRepository(database *gorm.DB) *ProductRepository {
	return &ProductRepository{database: database}
}

func (repository *ProductRepository) ListActive(ctx context.Context, filter ProductQuery) ([]models.Product, int64, error) {
	query := repository.database.WithContext(ctx).Model(&models.Product{}).Where("active = ?", true)
	if filter.CategoryID > 0 {
		query = query.Where("category_id = ?", filter.CategoryID)
	}
	if filter.MinPriceCents != nil {
		query = query.Where("price_cents >= ?", *filter.MinPriceCents)
	}
	if filter.MaxPriceCents != nil {
		query = query.Where("price_cents <= ?", *filter.MaxPriceCents)
	}
	if filter.Search != "" {
		query = query.Where("name LIKE ?", "%"+filter.Search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortColumn := "created_at"
	switch filter.Sort {
	case "name":
		sortColumn = "name"
	case "price":
		sortColumn = "price_cents"
	}
	direction := "DESC"
	if strings.EqualFold(filter.Order, "asc") {
		direction = "ASC"
	}

	var products []models.Product
	err := query.Preload("Category").Preload("Images", productImageOrder).Order(sortColumn + " " + direction).Limit(filter.Limit).Offset(filter.Offset).Find(&products).Error
	return products, total, err
}

func (repository *ProductRepository) GetActiveByID(ctx context.Context, productID uint) (*models.Product, error) {
	var product models.Product
	err := repository.database.WithContext(ctx).Preload("Category").Preload("Images", productImageOrder).Where("id = ? AND active = ?", productID, true).First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func productImageOrder(database *gorm.DB) *gorm.DB {
	return database.Order("position ASC, id ASC")
}

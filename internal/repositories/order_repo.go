package repositories

import (
	"context"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type OrderRepository struct{ database *gorm.DB }

func NewOrderRepository(database *gorm.DB) *OrderRepository {
	return &OrderRepository{database: database}
}

func (r *OrderRepository) ListByUserID(ctx context.Context, userID uint) ([]models.Order, error) {
	var orders []models.Order
	err := r.database.WithContext(ctx).Preload("Items").Where("user_id = ?", userID).Order("created_at DESC").Find(&orders).Error
	return orders, err
}

func (r *OrderRepository) GetByIDForUser(ctx context.Context, orderID, userID uint) (*models.Order, error) {
	var order models.Order
	if err := r.database.WithContext(ctx).Preload("Items").Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

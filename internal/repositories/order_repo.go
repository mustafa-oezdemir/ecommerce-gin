package repositories

import (
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

type OrderRepository struct{}

func NewOrderRepository() *OrderRepository { return &OrderRepository{} }

func (r *OrderRepository) ListByUserID(userID uint) ([]models.Order, error) {
	var orders []models.Order
	err := db.DB.Preload("Items").Where("user_id = ?", userID).Order("created_at DESC").Find(&orders).Error
	return orders, err
}

func (r *OrderRepository) GetByIDForUser(orderID, userID uint) (*models.Order, error) {
	var order models.Order
	if err := db.DB.Preload("Items").Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

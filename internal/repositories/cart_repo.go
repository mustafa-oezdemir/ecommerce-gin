package repositories

import (
	"context"
	"errors"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type CartRepository struct{ database *gorm.DB }

func NewCartRepository(database *gorm.DB) *CartRepository {
	return &CartRepository{database: database}
}

func (r *CartRepository) GetOrCreateCart(ctx context.Context, userID uint) (*models.Cart, error) {
	database := r.database.WithContext(ctx)
	var cart models.Cart
	err := database.Preload("Items.Product.Category").Where("user_id = ?", userID).First(&cart).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cart = models.Cart{UserID: userID}
		if createErr := database.Create(&cart).Error; createErr != nil {
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				if loadErr := database.Preload("Items.Product.Category").Where("user_id = ?", userID).First(&cart).Error; loadErr != nil {
					return nil, loadErr
				}
				return &cart, nil
			}
			return nil, createErr
		}
		return &cart, nil
	}
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

func (r *CartRepository) AddItem(ctx context.Context, cartID uint, productID uint, qty int) error {
	database := r.database.WithContext(ctx)
	var item models.CartItem
	err := database.Unscoped().Where("cart_id = ? AND product_id = ?", cartID, productID).First(&item).Error

	if err == nil {
		if item.DeletedAt.Valid {
			return database.Unscoped().Model(&item).Updates(map[string]interface{}{
				"deleted_at": nil,
				"quantity":   qty,
			}).Error
		}
		item.Quantity += qty
		return database.Save(&item).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	item = models.CartItem{
		CartID:    cartID,
		ProductID: productID,
		Quantity:  qty,
	}
	return database.Create(&item).Error
}

func (r *CartRepository) ClearCart(ctx context.Context, cartID uint) error {
	return r.database.WithContext(ctx).Where("cart_id = ?", cartID).Delete(&models.CartItem{}).Error
}

func (r *CartRepository) UpdateQuantityForUser(ctx context.Context, userID, itemID uint, quantity int) (bool, error) {
	database := r.database.WithContext(ctx)
	var cart models.Cart
	if err := database.Where("user_id = ?", userID).First(&cart).Error; err != nil {
		return false, err
	}
	result := database.Model(&models.CartItem{}).Where("id = ? AND cart_id = ?", itemID, cart.ID).Update("quantity", quantity)
	return result.RowsAffected == 1, result.Error
}

func (r *CartRepository) RemoveItemForUser(ctx context.Context, userID, itemID uint) (bool, error) {
	database := r.database.WithContext(ctx)
	var cart models.Cart
	if err := database.Where("user_id = ?", userID).First(&cart).Error; err != nil {
		return false, err
	}
	result := database.Where("id = ? AND cart_id = ?", itemID, cart.ID).Delete(&models.CartItem{})
	return result.RowsAffected == 1, result.Error
}

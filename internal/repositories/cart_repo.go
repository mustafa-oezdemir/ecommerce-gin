package repositories

import (
	"errors"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type CartRepository struct{}

func NewCartRepository() *CartRepository {
	return &CartRepository{}
}

func (r *CartRepository) GetOrCreateCart(userID uint) (*models.Cart, error) {
	var cart models.Cart
	err := db.DB.Preload("Items.Product.Category").Where("user_id = ?", userID).First(&cart).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cart = models.Cart{UserID: userID}
		if err := db.DB.Create(&cart).Error; err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

func (r *CartRepository) AddItem(cartID uint, productID uint, qty int) error {
	var item models.CartItem
	err := db.DB.Where("cart_id = ? AND product_id = ?", cartID, productID).First(&item).Error

	if err == nil {
		item.Quantity += qty
		return db.DB.Save(&item).Error
	}

	item = models.CartItem{
		CartID:    cartID,
		ProductID: productID,
		Quantity:  qty,
	}
	return db.DB.Create(&item).Error
}

func (r *CartRepository) ClearCart(cartID uint) error {
	return db.DB.Where("cart_id = ?", cartID).Delete(&models.CartItem{}).Error
}

func (r *CartRepository) UpdateQuantityForUser(userID, itemID uint, quantity int) (bool, error) {
	var cart models.Cart
	if err := db.DB.Where("user_id = ?", userID).First(&cart).Error; err != nil {
		return false, err
	}
	result := db.DB.Model(&models.CartItem{}).Where("id = ? AND cart_id = ?", itemID, cart.ID).Update("quantity", quantity)
	return result.RowsAffected == 1, result.Error
}

func (r *CartRepository) RemoveItemForUser(userID, itemID uint) (bool, error) {
	var cart models.Cart
	if err := db.DB.Where("user_id = ?", userID).First(&cart).Error; err != nil {
		return false, err
	}
	result := db.DB.Where("id = ? AND cart_id = ?", itemID, cart.ID).Delete(&models.CartItem{})
	return result.RowsAffected == 1, result.Error
}

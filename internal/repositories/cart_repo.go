package repositories

import (
    "github.com/mustafa/ecommerce-gin/internal/db"
    "github.com/mustafa/ecommerce-gin/internal/models"
)

type CartRepository struct{}

func NewCartRepository() *CartRepository {
    return &CartRepository{}
}

func (r *CartRepository) GetOrCreateCart(userID uint) (*models.Cart, error) {
    var cart models.Cart
    err := db.DB.Preload("Items.Product").Where("user_id = ?", userID).First(&cart).Error
    if err != nil {
        cart = models.Cart{UserID: userID}
        if err := db.DB.Create(&cart).Error; err != nil {
            return nil, err
        }
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

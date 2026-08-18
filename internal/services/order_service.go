package services

import (
    "errors"

    "github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

type OrderService struct{}

func NewOrderService() *OrderService {
    return &OrderService{}
}

func (s *OrderService) CreateOrder(user models.User) (*models.Order, error) {
    // Cart'ı al
    var cart models.Cart
    if err := db.DB.Preload("Items.Product").Where("user_id = ?", user.ID).First(&cart).Error; err != nil {
        return nil, errors.New("cart not found")
    }

    if len(cart.Items) == 0 {
        return nil, errors.New("cart is empty")
    }

    // Sipariş oluştur
    order := models.Order{
        UserID: user.ID,
        Status: "pending",
    }
    if err := db.DB.Create(&order).Error; err != nil {
        return nil, err
    }

    // OrderItem oluştur + stok düş
    for _, item := range cart.Items {
        orderItem := models.OrderItem{
            OrderID:   order.ID,
            ProductID: item.ProductID,
            Quantity:  item.Quantity,
            Price:     item.Product.Price,
        }
        if err := db.DB.Create(&orderItem).Error; err != nil {
            return nil, err
        }

        // Stok düş
        item.Product.Stock -= item.Quantity
        db.DB.Save(&item.Product)
    }

    // Sepeti temizle
    db.DB.Where("cart_id = ?", cart.ID).Delete(&models.CartItem{})

    return &order, nil
}

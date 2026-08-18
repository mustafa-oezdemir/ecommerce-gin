package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/mustafa/ecommerce-gin/internal/db"
    "github.com/mustafa/ecommerce-gin/internal/models"
)

type ShopHandler struct{}

func NewShopHandler() *ShopHandler {
    return &ShopHandler{}
}

func (h *ShopHandler) Home(c *gin.Context) {
    var products []models.Product
    db.DB.Limit(8).Find(&products)
    c.HTML(http.StatusOK, "product_list.tmpl", gin.H{
        "Title":    "Home",
        "Products": products,
    })
}

func (h *ShopHandler) ListProducts(c *gin.Context) {
    var products []models.Product
    db.DB.Find(&products)
    c.HTML(http.StatusOK, "product_list.tmpl", gin.H{
        "Title":    "Products",
        "Products": products,
    })
}

func (h *ShopHandler) ProductDetail(c *gin.Context) {
    id := c.Param("id")
    var product models.Product
    if err := db.DB.First(&product, id).Error; err != nil {
        c.String(http.StatusNotFound, "Product not found")
        return
    }
    c.HTML(http.StatusOK, "product_detail.tmpl", gin.H{
        "Product": product,
    })
}

// Cart & Checkout için basit session/cookie tabanlı sepet mantığını sonra ekleyebiliriz.
func (h *ShopHandler) AddToCart(c *gin.Context) {
    // TODO: basit cart implementasyonu
    c.String(http.StatusOK, "Added to cart")
}

func (h *ShopHandler) ViewCart(c *gin.Context) {
    c.HTML(http.StatusOK, "cart.tmpl", gin.H{})
}

func (h *ShopHandler) Checkout(c *gin.Context) {
    c.String(http.StatusOK, "Checkout complete")
}

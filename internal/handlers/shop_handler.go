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


func (h *ShopHandler) AddToCart(c *gin.Context) {
    userAny, exists := c.Get("currentUser")
    if !exists {
        c.Redirect(http.StatusFound, "/login")
        return
    }
    user := userAny.(models.User)

    productID, _ := strconv.Atoi(c.Param("id"))

    err := h.cartService.AddToCart(user, uint(productID), 1)
    if err != nil {
        c.String(http.StatusInternalServerError, "Error adding to cart")
        return
    }

    c.Redirect(http.StatusFound, "/cart")
}

func (h *ShopHandler) ViewCart(c *gin.Context) {
    userAny, exists := c.Get("currentUser")
    if !exists {
        c.Redirect(http.StatusFound, "/login")
        return
    }
    user := userAny.(models.User)

    cart, err := h.cartService.GetCart(user)
    if err != nil {
        c.String(http.StatusInternalServerError, "Error loading cart")
        return
    }

    c.HTML(http.StatusOK, "cart.tmpl", gin.H{
        "Items": cart.Items,
    })
}

func (h *ShopHandler) Checkout(c *gin.Context) {
    userAny, exists := c.Get("currentUser")
    if !exists {
        c.Redirect(http.StatusFound, "/login")
        return
    }
    user := userAny.(models.User)

    order, err := h.orderService.CreateOrder(user)
    if err != nil {
        c.String(http.StatusBadRequest, err.Error())
        return
    }

    c.HTML(http.StatusOK, "order_success.tmpl", gin.H{
        "Order": order,
    })
}


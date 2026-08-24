package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
)

type ShopHandler struct {
	cartService  *services.CartService
	orderService *services.OrderService
}

func NewShopHandler() *ShopHandler {
	return &ShopHandler{
		cartService:  services.NewCartService(),
		orderService: services.NewOrderService(),
	}
}

func (h *ShopHandler) Home(c *gin.Context) {
	c.HTML(http.StatusOK, "product_list.tmpl", gin.H{})
}

func (h *ShopHandler) ListProducts(c *gin.Context) {
	// Burada ürün listeleme kodun zaten vardı
}

func (h *ShopHandler) ProductDetail(c *gin.Context) {
	// Burada ürün detay kodun zaten vardı
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

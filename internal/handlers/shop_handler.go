package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ShopHandler struct {
	cartService  *services.CartService
	orderService *services.OrderService
	mailService  *services.MailService
}

func NewShopHandler() *ShopHandler {
	return &ShopHandler{cartService: services.NewCartService(), orderService: services.NewOrderService(), mailService: services.NewMailServiceFromEnv()}
}

func (h *ShopHandler) Home(c *gin.Context)         { h.renderProducts(c) }
func (h *ShopHandler) ListProducts(c *gin.Context) { h.renderProducts(c) }

func (h *ShopHandler) renderProducts(c *gin.Context) {
	var products []models.Product
	selectedCategoryID := uint(0)
	if categoryID, err := strconv.ParseUint(c.Query("category"), 10, 64); err == nil && categoryID > 0 {
		selectedCategoryID = uint(categoryID)
	}
	query := db.DB.Preload("Category").Where("active = ?", true).Order("created_at DESC")
	if selectedCategoryID > 0 {
		query = query.Where("category_id = ?", selectedCategoryID)
	}
	if err := query.Find(&products).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	var categories []models.Category
	if err := db.DB.Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	c.HTML(http.StatusOK, "product_list.tmpl", viewData(c, gin.H{"Products": products, "Categories": categories, "CategoryID": selectedCategoryID}))
}

func (h *ShopHandler) ProductDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var product models.Product
	if err := db.DB.Preload("Category").Preload("Images").Preload("Reviews", func(database *gorm.DB) *gorm.DB {
		return database.Order("created_at DESC")
	}).Preload("Reviews.User").Where("id = ? AND active = ?", uint(id), true).First(&product).Error; err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	data := gin.H{"Product": product}
	if user, ok := middleware.CurrentUser(c); ok && user.Role == models.RoleCustomer {
		var purchaseCount int64
		if err := db.DB.WithContext(c.Request.Context()).Model(&models.OrderItem{}).
			Joins("JOIN orders ON orders.id = order_items.order_id").
			Where("order_items.product_id = ? AND orders.user_id = ? AND orders.status <> ?", product.ID, user.ID, models.OrderStatusCancelled).
			Count(&purchaseCount).Error; err != nil {
			c.String(http.StatusInternalServerError, "Could not load product")
			return
		}
		data["CanReview"] = purchaseCount > 0
		if purchaseCount > 0 {
			var review models.ProductReview
			if err := db.DB.WithContext(c.Request.Context()).Where("product_id = ? AND user_id = ?", product.ID, user.ID).First(&review).Error; err == nil {
				data["ExistingReview"] = &review
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				c.String(http.StatusInternalServerError, "Could not load product")
				return
			}
		}
	}
	c.HTML(http.StatusOK, "product_detail.tmpl", viewData(c, data))
}

func (h *ShopHandler) CreateProductReview(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user.Role != models.RoleCustomer {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.CreateProductReviewRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Rating must be between 1 and 5 and the review must be 3 to 2000 characters")
		return
	}
	var purchaseCount int64
	if err := db.DB.WithContext(c.Request.Context()).Model(&models.OrderItem{}).
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("order_items.product_id = ? AND orders.user_id = ? AND orders.status <> ?", uri.ID, user.ID, models.OrderStatusCancelled).
		Count(&purchaseCount).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not verify purchase")
		return
	}
	if purchaseCount == 0 {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	review := models.ProductReview{ProductID: uri.ID, UserID: user.ID, Rating: req.Rating, Comment: strings.TrimSpace(req.Comment)}
	if err := db.DB.WithContext(c.Request.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "product_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"rating", "comment", "updated_at"}),
	}).Create(&review).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not save review")
		return
	}
	c.Redirect(http.StatusFound, "/products/"+strconv.FormatUint(uint64(uri.ID), 10))
}

func (h *ShopHandler) AddToCart(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.AddToCartRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid quantity")
		return
	}
	if err := h.cartService.AddToCart(*user, uri.ID, req.Quantity); err != nil {
		c.String(http.StatusBadRequest, "Could not add product to cart")
		return
	}
	c.Redirect(http.StatusFound, "/cart")
}

func (h *ShopHandler) ViewCart(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	cart, err := h.cartService.GetCart(*user)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load cart")
		return
	}
	var totalCents int64
	for _, item := range cart.Items {
		totalCents += item.Product.PriceCents * int64(item.Quantity)
	}
	c.HTML(http.StatusOK, "cart.tmpl", viewData(c, gin.H{"Cart": cart, "Items": cart.Items, "TotalCents": totalCents}))
}

func (h *ShopHandler) UpdateCartItem(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.CartItemIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.UpdateQuantityRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid quantity")
		return
	}
	if err := h.cartService.UpdateQuantity(*user, uri.ID, req.Quantity); err != nil {
		if errors.Is(err, services.ErrCartItemNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusBadRequest, "Could not update cart")
		return
	}
	c.Redirect(http.StatusFound, "/cart")
}

func (h *ShopHandler) RemoveCartItem(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.CartItemIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.cartService.RemoveItem(*user, uri.ID); err != nil {
		if errors.Is(err, services.ErrCartItemNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusBadRequest, "Could not remove cart item")
		return
	}
	c.Redirect(http.StatusFound, "/cart")
}

func (h *ShopHandler) Checkout(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	order, err := h.orderService.CreateOrder(*user)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCartNotFound), errors.Is(err, services.ErrCartEmpty), errors.Is(err, services.ErrInvalidQuantity):
			h.recordCheckoutFailure("empty_cart")
			c.String(http.StatusBadRequest, "Cart cannot be checked out")
		case errors.Is(err, services.ErrProductUnavailable), errors.Is(err, services.ErrInsufficientStock):
			if errors.Is(err, services.ErrInsufficientStock) {
				h.recordCheckoutFailure("insufficient_stock")
			} else {
				h.recordCheckoutFailure("product_unavailable")
			}
			c.String(http.StatusConflict, "A product is unavailable")
		default:
			h.recordCheckoutFailure("internal_error")
			c.String(http.StatusInternalServerError, "Checkout failed")
		}
		return
	}
	if metric := metrics.Default(); metric != nil {
		metric.OrdersCreated.WithLabelValues(string(order.Status)).Inc()
		metric.OrderValueCents.Observe(float64(order.TotalCents))
	}
	go h.mailService.SendOrderCreated(*user, *order)
	c.HTML(http.StatusOK, "order_success.tmpl", viewData(c, gin.H{"Order": order}))
}

func (h *ShopHandler) recordCheckoutFailure(reason string) {
	if metric := metrics.Default(); metric != nil {
		metric.CheckoutFailures.WithLabelValues(reason).Inc()
	}
}

func (h *ShopHandler) ListOrders(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	orders, err := h.orderService.ListUserOrders(user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load orders")
		return
	}
	c.HTML(http.StatusOK, "order_list.tmpl", viewData(c, gin.H{"Orders": orders}))
}

func (h *ShopHandler) OrderDetail(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	order, err := h.orderService.GetUserOrder(user.ID, uint(id))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.HTML(http.StatusOK, "order_detail.tmpl", viewData(c, gin.H{"Order": order}))
}

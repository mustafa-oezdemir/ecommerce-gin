package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

type ShopHandler struct {
	database     *gorm.DB
	cartService  *services.CartService
	orderService *services.OrderService
	mailService  *services.MailService
	engagement   *services.ProductEngagementService
	listService  *services.ProductListService
}

func NewShopHandler(database *gorm.DB) *ShopHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	return &ShopHandler{database: database, cartService: services.NewCartService(database), orderService: services.NewOrderService(database), mailService: services.NewMailServiceFromEnv(), engagement: services.NewProductEngagementService(database), listService: services.NewProductListService(database)}
}

func (h *ShopHandler) Home(c *gin.Context)         { h.renderProducts(c) }
func (h *ShopHandler) ListProducts(c *gin.Context) { h.renderProducts(c) }

func (h *ShopHandler) renderProducts(c *gin.Context) {
	var products []models.Product
	selectedCategoryID := uint(0)
	if categoryID, err := strconv.ParseUint(c.Query("category"), 10, 64); err == nil && categoryID > 0 {
		selectedCategoryID = uint(categoryID)
	}
	query := h.database.WithContext(c.Request.Context()).Preload("Category").Where("active = ?", true).Order("created_at DESC")
	if selectedCategoryID > 0 {
		query = query.Where("category_id = ?", selectedCategoryID)
	}
	if err := query.Find(&products).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	productIDs := make([]uint, len(products))
	for i := range products {
		productIDs[i] = products[i].ID
	}
	ratings, err := h.engagement.ReviewSummaries(c.Request.Context(), productIDs)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load product ratings")
		return
	}
	favorites := map[uint]bool{}
	if user, ok := middleware.CurrentUser(c); ok {
		favorites, _ = h.engagement.FavoriteProductIDs(c.Request.Context(), user.ID)
	}
	var categories []models.Category
	if err := h.database.WithContext(c.Request.Context()).Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	c.HTML(http.StatusOK, "product_list.tmpl", viewData(c, gin.H{"Products": products, "Categories": categories, "CategoryID": selectedCategoryID, "Ratings": ratings, "Favorites": favorites}))
}

func (h *ShopHandler) ProductDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var product models.Product
	if err := h.database.WithContext(c.Request.Context()).Preload("Category").Where("id = ? AND active = ?", uint(id), true).First(&product).Error; err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		page = 1
	}
	reviews, err := h.engagement.Reviews(c.Request.Context(), product.ID, page, c.Query("sort"))
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load reviews")
		return
	}
	data := gin.H{"Product": product, "Reviews": reviews.Reviews, "ReviewSummary": reviews.Summary, "ReviewPage": reviews, "Sort": c.Query("sort")}
	if user, ok := middleware.CurrentUser(c); ok {
		data["IsFavorite"], _ = h.engagement.IsFavorite(c.Request.Context(), user.ID, product.ID)
		data["CanReview"], _ = h.engagement.HasPurchased(c.Request.Context(), user.ID, product.ID)
		data["UserReview"], _ = h.engagement.UserReview(c.Request.Context(), user.ID, product.ID)
		lists, _ := h.listService.List(c.Request.Context(), user.ID)
		customLists := make([]models.ProductList, 0, len(lists))
		for _, list := range lists {
			if list.SystemKey == nil {
				customLists = append(customLists, list)
			}
		}
		data["Lists"] = customLists
	}
	c.HTML(http.StatusOK, "product_detail.tmpl", viewData(c, data))
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
	if err := h.cartService.AddToCart(c.Request.Context(), *user, uri.ID, req.Quantity); err != nil {
		if errors.Is(err, services.ErrInvalidCartInput) || errors.Is(err, services.ErrProductNotFound) || errors.Is(err, services.ErrProductInactive) {
			c.String(http.StatusBadRequest, "Could not add product to cart")
			return
		}
		c.String(http.StatusInternalServerError, "Could not add product to cart")
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
	cart, err := h.cartService.GetCart(c.Request.Context(), *user)
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
	if err := h.cartService.UpdateQuantity(c.Request.Context(), *user, uri.ID, req.Quantity); err != nil {
		if errors.Is(err, services.ErrCartItemNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidCartInput) {
			c.String(http.StatusBadRequest, "Could not update cart")
			return
		}
		c.String(http.StatusInternalServerError, "Could not update cart")
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
	if err := h.cartService.RemoveItem(c.Request.Context(), *user, uri.ID); err != nil {
		if errors.Is(err, services.ErrCartItemNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidCartInput) {
			c.String(http.StatusBadRequest, "Could not remove cart item")
			return
		}
		c.String(http.StatusInternalServerError, "Could not remove cart item")
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
	order, err := h.orderService.CreateOrder(c.Request.Context(), *user)
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
	orders, err := h.orderService.ListUserOrders(c.Request.Context(), user.ID)
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
	order, err := h.orderService.GetUserOrder(c.Request.Context(), user.ID, uint(id))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.HTML(http.StatusOK, "order_detail.tmpl", viewData(c, gin.H{"Order": order}))
}

package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	shippingapi "github.com/mustafa-oezdemir/ecommerce-gin/internal/shipping"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

type ShopHandler struct {
	database     *gorm.DB
	cartService  *services.CartService
	orderService *services.OrderService
	engagement   *services.ProductEngagementService
	listService  *services.ProductListService
	shipping     *services.ShippingService
	discovery    *services.ProductDiscoveryService
}

func NewShopHandler(database *gorm.DB, shippingService *services.ShippingService) *ShopHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	if shippingService == nil {
		panic("handlers: shipping service is required")
	}
	return &ShopHandler{database: database, cartService: services.NewCartService(database), orderService: services.NewOrderService(database), engagement: services.NewProductEngagementService(database), listService: services.NewProductListService(database), shipping: shippingService, discovery: services.NewProductDiscoveryService(repositories.NewProductRepository(database))}
}

func (h *ShopHandler) Home(c *gin.Context)         { h.renderProducts(c) }
func (h *ShopHandler) ListProducts(c *gin.Context) { h.renderProducts(c) }

func (h *ShopHandler) renderProducts(c *gin.Context) {
	filters, err := parseProductFilters(c.Request.URL.Query())
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid product filters")
		return
	}
	result, err := h.discovery.Search(c.Request.Context(), filters.discoveryRequest())
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	productIDs := make([]uint, len(result.Products))
	for i := range result.Products {
		productIDs[i] = result.Products[i].ID
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
	filterView := newProductFilterView(c.Request.URL.Query(), filters, result)
	c.HTML(http.StatusOK, "products/index", viewData(c, gin.H{
		"Products": result.Products, "Categories": result.Options.Categories, "Ratings": ratings,
		"Favorites": favorites, "ProductFilters": filterView, "ProductResult": result,
	}))
}

func (h *ShopHandler) ProductDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var product models.Product
	if err := h.database.WithContext(c.Request.Context()).Preload("Category").Preload("Images", func(database *gorm.DB) *gorm.DB {
		return database.Order("position ASC, id ASC")
	}).Where("id = ? AND active = ?", uint(id), true).First(&product).Error; err != nil {
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
	data := gin.H{"Product": product, "Images": product.GalleryImages(), "Reviews": reviews.Reviews, "ReviewSummary": reviews.Summary, "ReviewPage": reviews, "Sort": c.Query("sort")}
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
	c.HTML(http.StatusOK, "products/view", viewData(c, data))
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
	c.HTML(http.StatusOK, "cart/index", viewData(c, gin.H{"Cart": cart, "Items": cart.Items, "TotalCents": totalCents}))
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
	c.HTML(http.StatusOK, "account/orders/index", viewData(c, gin.H{"PageTitle": "My Orders", "Orders": orders}))
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
	data := gin.H{"PageTitle": "Order Details", "Order": order}
	canCancel := order.Status == models.OrderStatusPaid || order.Status == models.OrderStatusPreparing || order.Status == models.OrderStatusReadyForShipping || order.Status == models.OrderStatusProcessing
	if h.shipping.Enabled() {
		shipment, shippingErr := h.shipping.RefreshOrderShipment(c.Request.Context(), order.ID, c.GetString(middleware.RequestIDKey))
		if shippingErr != nil && !errors.Is(shippingErr, shippingapi.ErrNotFound) {
			data["ShippingUnavailable"] = true
			slog.WarnContext(c.Request.Context(), "shipping status refresh failed; using cached status", "order_id", order.ID, "error", shippingErr)
		}
		if shipment == nil {
			shipment, shippingErr = h.shipping.CachedOrderShipment(c.Request.Context(), order.ID)
		}
		if shippingErr != nil {
			slog.WarnContext(c.Request.Context(), "shipping status cache lookup failed", "order_id", order.ID, "error", shippingErr)
		} else if shipment != nil {
			data["Shipment"] = shipment
			data["ShipmentTrackingURL"] = h.shipping.TrackingURL(shipment.TrackingNumber)
			data["ShipmentQRCodeURL"] = h.shipping.QRCodeURL(shipment.TrackingNumber)
			if order.Status == models.OrderStatusShipped && shipment.Status != "delivered" && shipment.Status != "cancelled" {
				canCancel = true
			}
			if timeline, timelineErr := h.shipping.Timeline(c.Request.Context(), shipment.TrackingNumber, c.GetString(middleware.RequestIDKey)); timelineErr == nil {
				data["ShipmentTimeline"] = timeline
			} else {
				data["ShippingUnavailable"] = true
				slog.WarnContext(c.Request.Context(), "shipping timeline unavailable", "order_id", order.ID, "error", timelineErr)
			}
		}
	} else if shipment, cacheErr := h.shipping.CachedOrderShipment(c.Request.Context(), order.ID); cacheErr == nil && shipment != nil {
		data["Shipment"] = shipment
	}
	if shipment, cacheErr := h.shipping.CachedOrderShipment(c.Request.Context(), order.ID); cacheErr == nil && shipment != nil && shipment.Status == "delivered" {
		data["CanConfirmDelivery"] = order.CustomerReceivedAt == nil
		data["CanRequestReturn"] = order.ReturnRequest == nil
	}
	if returnShipment, cacheErr := h.shipping.CachedReturnShipment(c.Request.Context(), order.ID); cacheErr == nil && returnShipment != nil {
		data["ReturnShipment"] = returnShipment
		data["ReturnTrackingURL"] = h.shipping.TrackingURL(returnShipment.TrackingNumber)
		data["ReturnQRCodeURL"] = h.shipping.QRCodeURL(returnShipment.TrackingNumber)
	}
	data["CanCancelOrder"] = canCancel
	c.HTML(http.StatusOK, "account/orders/view", viewData(c, data))
}

func (h *ShopHandler) ConfirmDelivery(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.shipping.ConfirmDelivery(c.Request.Context(), user.ID, uint(orderID), c.GetString(middleware.RequestIDKey)); err != nil {
		if errors.Is(err, services.ErrDeliveryNotConfirmed) {
			c.String(http.StatusConflict, "Delivery has not been confirmed by shipping")
			return
		}
		if errors.Is(err, services.ErrShippingDisabled) || errors.Is(err, shippingapi.ErrUnavailable) || errors.Is(err, shippingapi.ErrTimeout) {
			c.String(http.StatusServiceUnavailable, "Shipping service is temporarily unavailable")
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusInternalServerError, "Could not confirm delivery")
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/orders/"+strconv.FormatUint(orderID, 10))
}

func (h *ShopHandler) RequestReturn(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	items, err := returnItemInputs(c)
	if err != nil {
		c.String(http.StatusBadRequest, "Select at least one valid return item")
		return
	}
	request, err := h.shipping.RequestReturn(c.Request.Context(), user.ID, uint(orderID), models.ReturnReason(c.PostForm("reason")), c.PostForm("note"), items, c.GetString(middleware.RequestIDKey))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrDeliveryNotConfirmed):
			c.String(http.StatusConflict, "Returns are available after delivery")
		case errors.Is(err, services.ErrInvalidReturnRequest):
			c.String(http.StatusBadRequest, "Select a valid return reason")
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.AbortWithStatus(http.StatusNotFound)
		case errors.Is(err, services.ErrShippingDisabled), errors.Is(err, shippingapi.ErrUnavailable), errors.Is(err, shippingapi.ErrTimeout):
			c.String(http.StatusServiceUnavailable, "Shipping service is temporarily unavailable")
		default:
			c.String(http.StatusInternalServerError, "Could not create return shipment")
		}
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/orders/"+strconv.FormatUint(uint64(request.OrderID), 10))
}

func (h *ShopHandler) CancelOrder(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || orderID == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.shipping.CancelOrder(c.Request.Context(), user.ID, uint(orderID), c.GetString(middleware.RequestIDKey)); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.AbortWithStatus(http.StatusNotFound)
		case errors.Is(err, services.ErrInvalidTransition):
			c.String(http.StatusConflict, "This order can no longer be cancelled")
		case errors.Is(err, services.ErrShippingDisabled), errors.Is(err, shippingapi.ErrUnavailable), errors.Is(err, shippingapi.ErrTimeout):
			c.String(http.StatusServiceUnavailable, "Shipping service is temporarily unavailable")
		default:
			c.String(http.StatusInternalServerError, "Could not cancel order")
		}
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/orders/"+strconv.FormatUint(orderID, 10))
}

func returnItemInputs(c *gin.Context) ([]services.ReturnItemInput, error) {
	ids := c.PostFormArray("return_item_id")
	if len(ids) == 0 {
		return nil, errors.New("no return items")
	}
	items := make([]services.ReturnItemInput, 0, len(ids))
	for _, rawID := range ids {
		id, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || id == 0 {
			return nil, errors.New("invalid return item")
		}
		quantity, err := strconv.Atoi(c.PostForm("return_quantity_" + rawID))
		if err != nil || quantity < 1 {
			return nil, errors.New("invalid return quantity")
		}
		items = append(items, services.ReturnItemInput{OrderItemID: uint(id), Quantity: quantity})
	}
	return items, nil
}

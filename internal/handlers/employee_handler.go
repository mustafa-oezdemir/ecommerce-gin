package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

type EmployeeHandler struct {
	orderService *services.OrderService
	mailService  *services.MailService
}

func NewEmployeeHandler() *EmployeeHandler {
	return &EmployeeHandler{orderService: services.NewOrderService(), mailService: services.NewMailServiceFromEnv()}
}

func (h *EmployeeHandler) Dashboard(c *gin.Context) {
	var pendingOrders, processingOrders, lowStockProducts, outOfStockProducts int64
	db.DB.Model(&models.Order{}).Where("status = ?", models.OrderStatusPending).Count(&pendingOrders)
	db.DB.Model(&models.Order{}).Where("status = ?", models.OrderStatusProcessing).Count(&processingOrders)
	db.DB.Model(&models.Product{}).Where("active = ? AND stock BETWEEN ? AND ?", true, 1, 5).Count(&lowStockProducts)
	db.DB.Model(&models.Product{}).Where("active = ? AND stock = ?", true, 0).Count(&outOfStockProducts)
	c.HTML(http.StatusOK, "employee_dashboard.tmpl", viewData(c, gin.H{"PendingOrders": pendingOrders, "ProcessingOrders": processingOrders, "LowStockProducts": lowStockProducts, "OutOfStockProducts": outOfStockProducts}))
}

func (h *EmployeeHandler) ListProducts(c *gin.Context) {
	var products []models.Product
	if err := db.DB.Preload("Category").Order("created_at DESC").Find(&products).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	var categories []models.Category
	if err := db.DB.Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	c.HTML(http.StatusOK, "employee_products.tmpl", viewData(c, gin.H{"Products": products, "Categories": categories}))
}

func (h *EmployeeHandler) CreateProduct(c *gin.Context) {
	var req validation.CreateProductRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid product data")
		return
	}
	priceCents, err := validation.ParseCents(req.Price)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid product data")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.String(http.StatusBadRequest, "Invalid product data")
		return
	}
	var categoryID *uint
	if req.CategoryID != 0 {
		var category models.Category
		if err := db.DB.First(&category, req.CategoryID).Error; err != nil {
			c.String(http.StatusBadRequest, "Invalid product data")
			return
		}
		categoryID = &category.ID
	}
	product := models.Product{Name: name, Description: strings.TrimSpace(req.Description), PriceCents: priceCents, Stock: req.Stock, Active: true, CategoryID: categoryID}
	if err := db.DB.Create(&product).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not create product")
		return
	}
	c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) UpdateStock(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.UpdateStockRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid stock value")
		return
	}
	result := db.DB.Model(&models.Product{}).Where("id = ?", uri.ID).Update("stock", req.Stock)
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not update stock")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) UpdateProduct(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.CreateProductRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid product data")
		return
	}
	priceCents, err := validation.ParseCents(req.Price)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid product data")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.String(http.StatusBadRequest, "Invalid product data")
		return
	}
	var categoryID *uint
	if req.CategoryID != 0 {
		var category models.Category
		if err := db.DB.First(&category, req.CategoryID).Error; err != nil {
			c.String(http.StatusBadRequest, "Invalid product data")
			return
		}
		categoryID = &category.ID
	}
	result := db.DB.Model(&models.Product{}).Where("id = ?", uri.ID).Updates(map[string]any{"name": name, "description": strings.TrimSpace(req.Description), "price_cents": priceCents, "stock": req.Stock, "category_id": categoryID})
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not update product")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) DeactivateProduct(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	result := db.DB.Model(&models.Product{}).Where("id = ?", uri.ID).Update("active", false)
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not deactivate product")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) ListOrders(c *gin.Context) {
	var orders []models.Order
	if err := db.DB.Preload("Items").Preload("User").Order("created_at DESC").Find(&orders).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load orders")
		return
	}
	c.HTML(http.StatusOK, "employee_orders.tmpl", viewData(c, gin.H{"Orders": orders}))
}

func (h *EmployeeHandler) UpdateOrderStatus(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.UpdateOrderStatusRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid order status")
		return
	}
	err := h.orderService.UpdateStatus(uri.ID, models.OrderStatus(req.Status))
	if err != nil {
		if errors.Is(err, services.ErrInvalidTransition) {
			c.String(http.StatusConflict, "Invalid order status transition")
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusInternalServerError, "Could not update order")
		return
	}
	var order models.Order
	if err := db.DB.Preload("User").First(&order, uri.ID).Error; err == nil {
		go h.mailService.SendOrderStatusChanged(order.User, order)
	}
	c.Redirect(http.StatusFound, "/employee/orders")
}

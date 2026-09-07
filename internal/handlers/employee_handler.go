package handlers

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

type EmployeeHandler struct {
	database     *gorm.DB
	orderService *services.OrderService
	mailService  *services.MailService
	imageStore   *uploads.ImageStore
}

func NewEmployeeHandler(database *gorm.DB, imageStore *uploads.ImageStore) *EmployeeHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	if imageStore == nil {
		panic("handlers: product image store is required")
	}
	return &EmployeeHandler{database: database, orderService: services.NewOrderService(database), mailService: services.NewMailServiceFromEnv(), imageStore: imageStore}
}

func (h *EmployeeHandler) Dashboard(c *gin.Context) {
	database := h.database.WithContext(c.Request.Context())
	var pendingOrders, processingOrders, lowStockProductCount, outOfStockProducts int64
	queries := []func() error{
		func() error {
			return applyPendingOrders(database.Model(&models.Order{})).Count(&pendingOrders).Error
		},
		func() error {
			return database.Model(&models.Order{}).Where("status = ?", models.OrderStatusProcessing).Count(&processingOrders).Error
		},
		func() error {
			return applyLowStockProducts(database.Model(&models.Product{})).Count(&lowStockProductCount).Error
		},
		func() error {
			return database.Model(&models.Product{}).Where("active = ? AND stock = ?", true, 0).Count(&outOfStockProducts).Error
		},
	}
	for _, query := range queries {
		if err := query(); err != nil {
			c.String(http.StatusInternalServerError, "Could not load dashboard")
			return
		}
	}
	c.HTML(http.StatusOK, "employee/dashboard/index", viewData(c, gin.H{"PendingOrders": pendingOrders, "ProcessingOrders": processingOrders, "LowStockProducts": lowStockProductCount, "OutOfStockProducts": outOfStockProducts}))
}

func (h *EmployeeHandler) ListProducts(c *gin.Context) {
	search := strings.TrimSpace(c.Query("q"))
	if searchRunes := []rune(search); len(searchRunes) > 100 {
		search = string(searchRunes[:100])
	}
	selectedCategoryID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("category_id")), 10, 64)
	selectedAvailability := strings.ToLower(strings.TrimSpace(c.DefaultQuery("availability", "all")))
	if !slices.Contains([]string{"all", "active", "inactive"}, selectedAvailability) {
		selectedAvailability = "all"
	}
	selectedStockStatus := normalizeStockStatus(c.Query("stock_status"))

	var products []models.Product
	query := h.database.WithContext(c.Request.Context()).Preload("Category").Preload("Brand").Preload("Variants").Preload("Images", func(database *gorm.DB) *gorm.DB {
		return database.Order("position ASC, id ASC")
	}).Order("products.created_at DESC")
	if search != "" {
		query = query.Where("products.name LIKE ?", "%"+search+"%")
	}
	if selectedCategoryID > 0 {
		query = query.Where("products.category_id = ?", selectedCategoryID)
	}
	switch selectedAvailability {
	case "active":
		query = query.Where("products.active = ?", true)
	case "inactive":
		query = query.Where("products.active = ?", false)
	}
	if selectedStockStatus == stockStatusLow {
		query = applyLowStockProducts(query)
	}
	if err := query.Find(&products).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	var categories []models.Category
	if err := h.database.WithContext(c.Request.Context()).Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	data := gin.H{
		"PageTitle":            "Products",
		"Products":             products,
		"Categories":           categories,
		"ImageMaxMB":           (h.imageStore.MaxBytes() + (1 << 20) - 1) / (1 << 20),
		"ImageLimit":           maxProductImages,
		"Search":               search,
		"SelectedCategoryID":   uint(selectedCategoryID),
		"SelectedAvailability": selectedAvailability,
		"SelectedStockStatus":  selectedStockStatus,
		"LowStockFilter":       selectedStockStatus == stockStatusLow,
		"DashboardURL":         managementDashboardURL(c),
	}
	if editID, err := strconv.ParseUint(strings.TrimSpace(c.Query("edit")), 10, 64); err == nil && editID > 0 {
		for index := range products {
			if products[index].ID == uint(editID) {
				data["EditProduct"] = &products[index]
				break
			}
		}
	}
	switch c.Query("status") {
	case "images-added":
		data["Success"] = "Product images were added successfully."
	case "image-deleted":
		data["Success"] = "The product image was deleted."
	case "cover-updated":
		data["Success"] = "The cover image was updated."
	}
	c.HTML(http.StatusOK, "employee/products/index", viewData(c, data))
}

func (h *EmployeeHandler) NewProduct(c *gin.Context) {
	categories, ok := h.productCategories(c)
	if !ok {
		return
	}
	c.HTML(http.StatusOK, "employee/products/create", viewData(c, gin.H{"PageTitle": "Add Product", "Product": models.Product{Active: true}, "Categories": categories, "ImageMaxMB": (h.imageStore.MaxBytes() + (1 << 20) - 1) / (1 << 20), "ImageLimit": maxProductImages}))
}

func (h *EmployeeHandler) ViewProduct(c *gin.Context) {
	product, ok := h.productFromParam(c)
	if !ok {
		return
	}
	c.HTML(http.StatusOK, "employee/products/view", viewData(c, gin.H{"PageTitle": "Product Details", "Product": product}))
}

func (h *EmployeeHandler) EditProduct(c *gin.Context) {
	product, ok := h.productFromParam(c)
	if !ok {
		return
	}
	categories, ok := h.productCategories(c)
	if !ok {
		return
	}
	c.HTML(http.StatusOK, "employee/products/edit", viewData(c, gin.H{"PageTitle": "Edit Product", "Product": product, "Categories": categories, "ImageMaxMB": (h.imageStore.MaxBytes() + (1 << 20) - 1) / (1 << 20), "ImageLimit": maxProductImages}))
}

func (h *EmployeeHandler) productFromParam(c *gin.Context) (*models.Product, bool) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return nil, false
	}
	var product models.Product
	if err := h.database.WithContext(c.Request.Context()).Preload("Category").Preload("Brand").Preload("Variants").Preload("Images", func(database *gorm.DB) *gorm.DB { return database.Order("position ASC, id ASC") }).First(&product, uri.ID).Error; err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return nil, false
	}
	return &product, true
}

func (h *EmployeeHandler) productCategories(c *gin.Context) ([]models.Category, bool) {
	var categories []models.Category
	if err := h.database.WithContext(c.Request.Context()).Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load categories")
		return nil, false
	}
	return categories, true
}

func (h *EmployeeHandler) CreateProduct(c *gin.Context) {
	h.limitImageUpload(c)
	var req validation.CreateProductRequest
	if err := c.ShouldBind(&req); err != nil {
		if isUploadTooLarge(err) {
			c.String(http.StatusRequestEntityTooLarge, "Product image is too large")
			return
		}
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
		if err := h.database.WithContext(c.Request.Context()).First(&category, req.CategoryID).Error; err != nil {
			c.String(http.StatusBadRequest, "Invalid product data")
			return
		}
		categoryID = &category.ID
	}
	imageFilenames, err := h.saveProductImages(c, false, maxProductImages)
	if err != nil {
		h.respondToImageError(c, err)
		return
	}
	product := models.Product{Name: name, Description: strings.TrimSpace(req.Description), PriceCents: priceCents, Stock: req.Stock, Active: true, CategoryID: categoryID}
	if len(imageFilenames) > 0 {
		product.ImageFilename = imageFilenames[0]
		product.Images = make([]models.ProductImage, 0, len(imageFilenames))
		for position, filename := range imageFilenames {
			product.Images = append(product.Images, models.ProductImage{Filename: filename, Position: uint(position)})
		}
	}
	if err := h.database.WithContext(c.Request.Context()).Create(&product).Error; err != nil {
		h.cleanupProductImages(imageFilenames)
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
	result := h.database.WithContext(c.Request.Context()).Model(&models.Product{}).Where("id = ?", uri.ID).Update("stock", req.Stock)
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
		if err := h.database.WithContext(c.Request.Context()).First(&category, req.CategoryID).Error; err != nil {
			c.String(http.StatusBadRequest, "Invalid product data")
			return
		}
		categoryID = &category.ID
	}
	result := h.database.WithContext(c.Request.Context()).Model(&models.Product{}).Where("id = ?", uri.ID).Updates(map[string]any{"name": name, "description": strings.TrimSpace(req.Description), "price_cents": priceCents, "stock": req.Stock, "category_id": categoryID})
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
	result := h.database.WithContext(c.Request.Context()).Model(&models.Product{}).Where("id = ?", uri.ID).Update("active", false)
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

func (h *EmployeeHandler) ActivateProduct(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	result := h.database.WithContext(c.Request.Context()).Model(&models.Product{}).Where("id = ?", uri.ID).Update("active", true)
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not activate product")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusSeeOther, "/employee/products?edit="+strconv.FormatUint(uint64(uri.ID), 10)+"#edit-product")
}

func (h *EmployeeHandler) ListOrders(c *gin.Context) {
	userSearch := strings.TrimSpace(c.Query("user"))
	if searchRunes := []rune(userSearch); len(searchRunes) > 100 {
		userSearch = string(searchRunes[:100])
	}
	selectedStatus := normalizeManagementOrderStatus(c.Query("status"))
	selectedSort := strings.ToLower(strings.TrimSpace(c.DefaultQuery("sort", "id_desc")))
	sortOptions := map[string]string{
		"id_desc":    "orders.id DESC",
		"id_asc":     "orders.id ASC",
		"user_asc":   "COALESCE(NULLIF(users.name, ''), users.email) ASC, orders.id DESC",
		"user_desc":  "COALESCE(NULLIF(users.name, ''), users.email) DESC, orders.id DESC",
		"total_desc": "orders.total_cents DESC, orders.id DESC",
		"total_asc":  "orders.total_cents ASC, orders.id DESC",
	}
	orderBy, validSort := sortOptions[selectedSort]
	if !validSort {
		selectedSort = "id_desc"
		orderBy = sortOptions[selectedSort]
	}

	var orders []models.Order
	query := h.database.WithContext(c.Request.Context()).Preload("Items").Preload("User").Preload("Payment").Preload("Shipment").Joins("JOIN users ON users.id = orders.user_id").Order(orderBy)
	if userSearch != "" {
		like := "%" + userSearch + "%"
		query = query.Where("(users.name LIKE ? OR users.email LIKE ?)", like, like)
	}
	if selectedStatus == models.OrderStatusPending {
		query = applyPendingOrders(query)
	} else if selectedStatus != "" {
		query = query.Where("orders.status = ?", selectedStatus)
	}
	if err := query.Find(&orders).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load orders")
		return
	}
	c.HTML(http.StatusOK, "employee/orders/index", viewData(c, gin.H{
		"PageTitle":      "Orders",
		"Orders":         orders,
		"Statuses":       managementOrderStatuses,
		"UserSearch":     userSearch,
		"SelectedStatus": string(selectedStatus),
		"SelectedSort":   selectedSort,
		"PendingFilter":  selectedStatus == models.OrderStatusPending,
		"DashboardURL":   managementDashboardURL(c),
	}))
}

func (h *EmployeeHandler) ViewOrder(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var order models.Order
	if err := h.database.WithContext(c.Request.Context()).Preload("User").Preload("Payment").Preload("Shipment").Preload("Items.Product").Preload("Addresses").First(&order, uri.ID).Error; err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.HTML(http.StatusOK, "employee/orders/view", viewData(c, gin.H{"PageTitle": "Order Details", "Order": order}))
}

func managementDashboardURL(c *gin.Context) string {
	if user, ok := middleware.CurrentUser(c); ok && user.Role == models.RoleAdmin {
		return "/admin/dashboard"
	}
	return "/employee/dashboard"
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
	err := h.orderService.UpdateStatus(c.Request.Context(), uri.ID, models.OrderStatus(req.Status))
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
	if err := h.database.WithContext(c.Request.Context()).Preload("User").First(&order, uri.ID).Error; err == nil {
		go h.mailService.SendOrderStatusChanged(order.User, order)
	}
	c.Redirect(http.StatusFound, "/employee/orders")
}

package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/logging"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct {
	logReader *logging.Reader
}

func NewAdminHandler(logReader *logging.Reader) *AdminHandler {
	if logReader == nil {
		panic("handlers: application log reader is required")
	}
	return &AdminHandler{logReader: logReader}
}

func (h *AdminHandler) Dashboard(c *gin.Context) {
	var customers, employees, products, pendingOrders, lowStock, totalOrders int64
	var revenue struct{ Total int64 }
	db.DB.Model(&models.User{}).Where("role = ?", models.RoleCustomer).Count(&customers)
	db.DB.Model(&models.User{}).Where("role = ?", models.RoleEmployee).Count(&employees)
	db.DB.Model(&models.Product{}).Count(&products)
	db.DB.Model(&models.Product{}).Where("active = ? AND stock <= ?", true, 5).Count(&lowStock)
	db.DB.Model(&models.Order{}).Where("status = ?", models.OrderStatusPending).Count(&pendingOrders)
	db.DB.Model(&models.Order{}).Count(&totalOrders)
	db.DB.Model(&models.Order{}).Select("COALESCE(SUM(total_cents), 0) AS total").Where("status <> ?", models.OrderStatusCancelled).Scan(&revenue)
	c.HTML(http.StatusOK, "admin_dashboard.tmpl", viewData(c, gin.H{"Customers": customers, "Employees": employees, "Products": products, "PendingOrders": pendingOrders, "LowStock": lowStock, "TotalOrders": totalOrders, "RevenueCents": revenue.Total}))
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []models.User
	if err := db.DB.Order("created_at DESC").Find(&users).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load users")
		return
	}
	c.HTML(http.StatusOK, "admin_users.tmpl", viewData(c, gin.H{"Users": users}))
}

func (h *AdminHandler) Logs(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit < 1 {
		limit = 100
	}
	limit = min(limit, 500)
	level := strings.ToLower(strings.TrimSpace(c.DefaultQuery("level", "all")))
	search := strings.TrimSpace(c.Query("q"))
	if searchRunes := []rune(search); len(searchRunes) > 100 {
		search = string(searchRunes[:100])
	}
	snapshot, err := h.logReader.Read(logging.LogQuery{Limit: limit, Level: level, Search: search})
	if err != nil {
		if errors.Is(err, logging.ErrInvalidLogLevel) {
			level = "all"
			snapshot, err = h.logReader.Read(logging.LogQuery{Limit: limit, Search: search})
		}
		if err != nil {
			slog.Error("application logs could not be read", "error", err)
			c.String(http.StatusInternalServerError, "Could not load application logs")
			return
		}
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(http.StatusOK, "admin_logs.tmpl", viewData(c, gin.H{
		"Snapshot":    snapshot,
		"Level":       level,
		"Limit":       limit,
		"Search":      search,
		"AutoRefresh": c.Query("refresh") == "10",
	}))
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req validation.CreateUserRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid user data")
		return
	}
	name, email := strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Email))
	if name == "" || email == "" {
		c.String(http.StatusBadRequest, "Invalid user data")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not create user")
		return
	}
	user := models.User{Name: name, Email: email, Password: string(hash), Role: models.Role(req.Role)}
	if err := db.DB.Create(&user).Error; err != nil {
		c.String(http.StatusConflict, "Could not create user")
		return
	}
	c.Redirect(http.StatusFound, "/admin/users")
}

func (h *AdminHandler) ListOrders(c *gin.Context) {
	var orders []models.Order
	if err := db.DB.Preload("Items").Preload("User").Order("created_at DESC").Find(&orders).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load orders")
		return
	}
	c.HTML(http.StatusOK, "admin_orders.tmpl", viewData(c, gin.H{"Orders": orders}))
}

func (h *AdminHandler) ListCategories(c *gin.Context) {
	var categories []models.Category
	if err := db.DB.Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load categories")
		return
	}
	c.HTML(http.StatusOK, "admin_categories.tmpl", viewData(c, gin.H{"Categories": categories}))
}

func (h *AdminHandler) CreateCategory(c *gin.Context) {
	var req validation.CreateCategoryRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid category data")
		return
	}
	category := models.Category{Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description)}
	if category.Name == "" {
		c.String(http.StatusBadRequest, "Invalid category data")
		return
	}
	if err := db.DB.Create(&category).Error; err != nil {
		c.String(http.StatusConflict, "Could not create category")
		return
	}
	c.Redirect(http.StatusFound, "/admin/categories")
}

func (h *AdminHandler) DeleteCategory(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	result := db.DB.Delete(&models.Category{}, uri.ID)
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not delete category")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, "/admin/categories")
}

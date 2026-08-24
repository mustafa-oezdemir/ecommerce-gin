package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler { return &AdminHandler{} }

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

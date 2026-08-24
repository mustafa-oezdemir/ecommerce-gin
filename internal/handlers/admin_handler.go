package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
}

func (h *AdminHandler) Dashboard(c *gin.Context) {
	var userCount int64
	var productCount int64
	db.DB.Model(&models.User{}).Count(&userCount)
	db.DB.Model(&models.Product{}).Count(&productCount)

	c.HTML(http.StatusOK, "admin_dashboard.tmpl", gin.H{
		"UserCount":    userCount,
		"ProductCount": productCount,
	})
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []models.User
	db.DB.Find(&users)
	c.HTML(http.StatusOK, "admin_users.tmpl", gin.H{
		"Users": users,
	})
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	name := c.PostForm("name")
	email := c.PostForm("email")
	password := c.PostForm("password")
	role := c.PostForm("role")

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	user := models.User{
		Name:     name,
		Email:    email,
		Password: string(hash),
		Role:     models.Role(role),
	}

	if err := db.DB.Create(&user).Error; err != nil {
		c.String(http.StatusInternalServerError, "Error creating user")
		return
	}

	c.Redirect(http.StatusFound, "/admin/users")
}

func (h *AdminHandler) ListOrders(c *gin.Context) {
	var orders []models.Order
	db.DB.Preload("Items.Product").Preload("User").Find(&orders)

	c.HTML(http.StatusOK, "admin_orders.tmpl", gin.H{
		"Orders": orders,
	})
}

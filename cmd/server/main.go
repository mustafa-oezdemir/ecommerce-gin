package main

import (
    "html/template"
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/mustafa-oezdemir/ecommerce-gin/internal/config"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/handlers"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

func main() {
    cfg := config.Load()
    db.Init(cfg)

    r := gin.Default()

    // Templates
    r.LoadHTMLGlob("internal/web/templates/*.tmpl")
    r.Static("/static", "./internal/web/static")

    // Public routes (shop)
    shop := handlers.NewShopHandler()
    r.GET("/", shop.Home)
    r.GET("/products", shop.ListProducts)
    r.GET("/products/:id", shop.ProductDetail)
    r.POST("/cart/add/:id", shop.AddToCart)
    r.GET("/cart", shop.ViewCart)
    r.POST("/checkout", shop.Checkout)

    // Auth
    auth := handlers.NewAuthHandler()
    r.GET("/login", auth.ShowLogin)
    r.POST("/login", auth.Login)
    r.GET("/logout", auth.Logout)

    // Admin / Employee (protected)
    admin := handlers.NewAdminHandler()
    adminGroup := r.Group("/admin")
    adminGroup.Use(handlers.AuthRequired(models.RoleAdmin)) // pseudo
    {
        adminGroup.GET("/dashboard", admin.Dashboard)
        adminGroup.GET("/users", admin.ListUsers)
        adminGroup.POST("/users", admin.CreateUser)
    }

    employee := handlers.NewEmployeeHandler()
    empGroup := r.Group("/employee")
    empGroup.Use(handlers.AuthRequired(models.RoleEmployee))
    {
        empGroup.GET("/products", employee.ListProducts)
        empGroup.POST("/products", employee.CreateProduct)
        empGroup.POST("/products/:id/stock", employee.UpdateStock)
    }

    r.Run(":8080")
}

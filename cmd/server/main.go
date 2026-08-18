    package main

    import (
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

    r.LoadHTMLFiles(
    "internal/web/templates/layout.tmpl",
    "internal/web/templates/product_list.tmpl",
    "internal/web/templates/cart.tmpl",
    "internal/web/templates/order_success.tmpl",
    )
    r.Static("/static", "./internal/web/static")
        // Shop
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

        // Admin
        admin := handlers.NewAdminHandler()
        adminGroup := r.Group("/admin")
        adminGroup.Use(handlers.AuthRequired(models.RoleAdmin))
        {
            adminGroup.GET("/dashboard", admin.Dashboard)
            adminGroup.GET("/users", admin.ListUsers)
            adminGroup.POST("/users", admin.CreateUser)
            adminGroup.GET("/orders", admin.ListOrders)
        }

        // Employee
        employee := handlers.NewEmployeeHandler()
        employeeGroup := r.Group("/employee")
        employeeGroup.Use(handlers.AuthRequired(models.RoleEmployee))
        {
            employeeGroup.GET("/products", employee.ListProducts)
            employeeGroup.POST("/products", employee.CreateProduct)
            employeeGroup.POST("/products/:id/stock", employee.UpdateStock)
        }

        r.Run(":8080")
    }

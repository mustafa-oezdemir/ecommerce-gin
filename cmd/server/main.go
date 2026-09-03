package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/config"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/handlers"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
)

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)
	if err := db.Init(cfg); err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}

	r := gin.New()
	appMetrics := metrics.New(prometheus.DefaultRegisterer)
	if sqlDB, err := db.SQL(); err == nil {
		prometheus.MustRegister(collectors.NewDBStatsCollector(sqlDB, "ecommerce"))
	}
	appMetrics.HealthLive.Set(1)
	metrics.SetDefault(appMetrics)
	r.Use(gin.Recovery(), middleware.RequestID(), middleware.Metrics(appMetrics), middleware.RequestLogger(slog.Default()), middleware.SecurityHeaders(cfg.AppEnv == "production"))
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{Path: "/", MaxAge: 60 * 60 * 8, HttpOnly: true, Secure: cfg.SessionSecure, SameSite: http.SameSiteLaxMode})
	r.Use(sessions.Sessions("ecommerce_session", store))

	templates, err := web.ParseTemplates()
	if err != nil {
		panic(err)
	}

	r.SetHTMLTemplate(templates)
	r.Static("/static", "./internal/web/static")
	health := handlers.NewHealthHandler(db.DB, appMetrics)
	r.GET("/health/live", health.Live)
	r.GET("/health/ready", health.Ready)
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)
	// Shop
	shop := handlers.NewShopHandler()
	r.GET("/", shop.Home)
	r.GET("/products", shop.ListProducts)
	r.GET("/products/:id", shop.ProductDetail)
	customer := r.Group("")
	customer.Use(middleware.RequireAuth(), middleware.RequireRoles(models.RoleCustomer))
	customer.POST("/cart/add/:id", shop.AddToCart)
	customer.GET("/cart", shop.ViewCart)
	customer.POST("/cart/items/:id", shop.UpdateCartItem)
	customer.POST("/cart/items/:id/remove", shop.RemoveCartItem)
	customer.POST("/checkout", shop.Checkout)
	customer.GET("/account/orders", shop.ListOrders)
	customer.GET("/account/orders/:id", shop.OrderDetail)
	customer.GET("/account/purchases", shop.ListOrders)
	account := handlers.NewAccountHandler()
	customer.GET("/account", account.Show)
	customer.GET("/account/profile", account.Show)
	customer.POST("/account/profile", account.UpdateProfile)
	customer.POST("/account/password", account.ChangePassword)
	customer.GET("/account/lists", account.ListProductLists)
	customer.POST("/account/lists", account.CreateProductList)
	customer.GET("/account/lists/:id", account.ShowProductList)
	customer.POST("/account/lists/:id/products", account.AddProductToList)
	customer.POST("/account/lists/:id/products/:productID/remove", account.RemoveProductFromList)
	customer.POST("/account/lists/:id/delete", account.DeleteProductList)

	// Auth
	auth := handlers.NewAuthHandler()
	r.GET("/login", auth.ShowLogin)
	loginLimiter := middleware.NewLoginRateLimiter(10, time.Minute)
	r.POST("/login", loginLimiter.Middleware(), auth.Login)
	r.POST("/logout", middleware.RequireAuth(), auth.Logout)

	// Admin
	admin := handlers.NewAdminHandler()
	adminGroup := r.Group("/admin")
	adminGroup.Use(middleware.RequireAuth(), middleware.RequireRoles(models.RoleAdmin))
	{
		adminGroup.GET("/dashboard", admin.Dashboard)
		adminGroup.GET("/users", admin.ListUsers)
		adminGroup.POST("/users", admin.CreateUser)
		adminGroup.GET("/orders", admin.ListOrders)
		adminGroup.GET("/categories", admin.ListCategories)
		adminGroup.POST("/categories", admin.CreateCategory)
		adminGroup.POST("/categories/:id/delete", admin.DeleteCategory)
	}

	// Employee
	employee := handlers.NewEmployeeHandler()
	employeeGroup := r.Group("/employee")
	employeeGroup.Use(middleware.RequireAuth(), middleware.RequireRoles(models.RoleAdmin, models.RoleEmployee))
	{
		employeeGroup.GET("/dashboard", employee.Dashboard)
		employeeGroup.GET("/products", employee.ListProducts)
		employeeGroup.POST("/products", employee.CreateProduct)
		employeeGroup.POST("/products/:id", employee.UpdateProduct)
		employeeGroup.POST("/products/:id/deactivate", employee.DeactivateProduct)
		employeeGroup.POST("/products/:id/stock", employee.UpdateStock)
		employeeGroup.GET("/orders", employee.ListOrders)
		employeeGroup.POST("/orders/:id/status", employee.UpdateOrderStatus)
	}

	csrfMiddleware := csrf.Protect(cfg.CSRFKey, csrf.Secure(cfg.SessionSecure), csrf.HttpOnly(true), csrf.SameSite(csrf.SameSiteLaxMode), csrf.Path("/"), csrf.FieldName("_csrf"), csrf.RequestHeader("X-CSRF-Token"), csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Default().Warn("csrf validation failed", "reason", csrf.FailureReason(r))
		http.Error(w, "Forbidden", http.StatusForbidden)
	})))
	csrfProtectedHandler := csrfMiddleware(r)
	serverHandler := csrfProtectedHandler
	if cfg.AppEnv != "production" {
		serverHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			csrfProtectedHandler.ServeHTTP(w, csrf.PlaintextHTTPRequest(req))
		})
	}
	server := &http.Server{Addr: ":" + cfg.AppPort, Handler: serverHandler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	metricsServer := &http.Server{Addr: ":" + cfg.MetricsPort, Handler: promhttp.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("server listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	go func() {
		log.Printf("metrics listening on %s", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Printf("metrics shutdown failed: %v", err)
	}
	if err := db.Close(); err != nil {
		log.Printf("database close failed: %v", err)
	}
}

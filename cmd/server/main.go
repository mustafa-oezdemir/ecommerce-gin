package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/logging"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "application stopped: %v\n", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	cfg := config.Load()
	logRuntime, err := logging.New(logging.Config{
		Environment:   cfg.AppEnv,
		Level:         cfg.LogLevel,
		ConsoleFormat: cfg.LogConsoleFormat,
		FilePath:      cfg.LogFile,
		MaxSizeMB:     cfg.LogMaxSizeMB,
		MaxBackups:    cfg.LogMaxBackups,
		MaxAgeDays:    cfg.LogMaxAgeDays,
		Compress:      cfg.LogCompress,
		AddSource:     cfg.LogAddSource,
	})
	if err != nil {
		return fmt.Errorf("initialize logging: %w", err)
	}
	defer func() {
		if err := logRuntime.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close log file: %w", err))
		}
	}()
	slog.SetDefault(logRuntime.Logger)
	defer func() {
		if runErr != nil {
			slog.Error("application stopped with errors", "error", runErr)
		} else {
			slog.Info("application stopped cleanly")
		}
	}()
	gin.SetMode(cfg.GinMode)
	gin.DisableConsoleColor()
	gin.DefaultWriter = logging.NewWriter(slog.Default(), slog.LevelDebug, "gin")
	gin.DefaultErrorWriter = logging.NewWriter(slog.Default(), slog.LevelError, "gin")
	gin.DebugPrintRouteFunc = func(method, path, handler string, handlerCount int) {
		slog.Debug("route registered", "method", method, "route", path, "handler", handler, "handler_count", handlerCount)
	}
	slog.Info("logging initialized", "file", logRuntime.FilePath(), "minimum_level", cfg.LogLevel, "console_format", cfg.LogConsoleFormat)
	if err := db.Init(cfg); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close database: %w", err))
		}
	}()

	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return fmt.Errorf("configure trusted proxies: %w", err)
	}
	appMetrics := metrics.New(prometheus.DefaultRegisterer)
	if sqlDB, err := db.SQL(); err == nil {
		prometheus.MustRegister(collectors.NewDBStatsCollector(sqlDB, "ecommerce"))
	}
	appMetrics.HealthLive.Set(1)
	metrics.SetDefault(appMetrics)
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{Path: "/", MaxAge: 60 * 60 * 8, HttpOnly: true, Secure: cfg.SessionSecure, SameSite: http.SameSiteLaxMode})
	middleware.Install(r, middleware.StackConfig{
		Logger:          slog.Default(),
		MetricsRegistry: appMetrics,
		EnableHSTS:      cfg.AppEnv == "production",
		Session:         sessions.Sessions("ecommerce_session", store),
	})

	templates, err := web.ParseTemplates()
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
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
	requireAuth := middleware.RequireAuth(db.DB)
	r.GET("/", shop.Home)
	r.GET("/products", shop.ListProducts)
	r.GET("/products/:id", shop.ProductDetail)
	customer := r.Group("")
	customer.Use(requireAuth, middleware.RequireRoles(models.RoleCustomer))
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
	r.POST("/logout", requireAuth, auth.Logout)

	// Admin
	admin := handlers.NewAdminHandler()
	adminGroup := r.Group("/admin")
	adminGroup.Use(requireAuth, middleware.RequireRoles(models.RoleAdmin))
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
	employeeGroup.Use(requireAuth, middleware.RequireRoles(models.RoleAdmin, models.RoleEmployee))
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
	serverErrors := make(chan error, 2)
	go func() {
		slog.Info("application server listening", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- fmt.Errorf("application server: %w", err)
		}
	}()
	go func() {
		slog.Info("metrics server listening", "address", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- fmt.Errorf("metrics server: %w", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case receivedSignal := <-stop:
		slog.Info("shutdown signal received", "signal", receivedSignal.String())
	case err := <-serverErrors:
		slog.Error("server failure triggered shutdown", "error", err)
		runErr = errors.Join(runErr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown application server: %w", err))
	}
	if err := metricsServer.Shutdown(ctx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown metrics server: %w", err))
	}
	return runErr
}

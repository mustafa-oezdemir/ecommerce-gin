package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/api"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/handlers"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/logging"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
	"gorm.io/gorm"
)

type RouterConfig struct {
	Environment    string
	TrustedProxies []string
	SessionSecret  string
	SessionSecure  bool
	CSRFKey        []byte
	SecurityKey    []byte
	Database       *gorm.DB
	Metrics        *metrics.Metrics
	Logger         *slog.Logger
	ImageStore     *uploads.ImageStore
	LogReader      *logging.Reader
}

func NewRouter(config RouterConfig) (http.Handler, error) {
	if config.Environment != "development" && config.Environment != "test" && config.Environment != "production" {
		return nil, fmt.Errorf("server: unsupported environment %q", config.Environment)
	}
	if len(config.SessionSecret) < 32 {
		return nil, errors.New("server: session secret must contain at least 32 characters")
	}
	if len(config.CSRFKey) != 32 {
		return nil, errors.New("server: CSRF key must contain exactly 32 bytes")
	}
	if len(config.SecurityKey) != 32 {
		return nil, errors.New("server: security encryption key must contain exactly 32 bytes")
	}
	if config.Database == nil {
		return nil, errors.New("server: database is required")
	}
	if config.Metrics == nil {
		return nil, errors.New("server: metrics registry is required")
	}
	if config.ImageStore == nil {
		return nil, errors.New("server: product image store is required")
	}
	if config.LogReader == nil {
		return nil, errors.New("server: application log reader is required")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	router := gin.New()
	router.MaxMultipartMemory = config.ImageStore.MaxBytes()
	if err := router.SetTrustedProxies(config.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.HandleMethodNotAllowed = true

	store := cookie.NewStore([]byte(config.SessionSecret))
	store.Options(sessions.Options{Path: "/", MaxAge: 60 * 60 * 8, HttpOnly: true, Secure: config.SessionSecure, SameSite: http.SameSiteLaxMode})
	middleware.Install(router, middleware.StackConfig{
		Logger:          config.Logger,
		MetricsRegistry: config.Metrics,
		EnableHSTS:      config.Environment == "production",
		Session:         sessions.Sessions("ecommerce_session", store),
	})

	templates, err := web.ParseTemplates()
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	router.SetHTMLTemplate(templates)
	router.Static("/static", "./internal/web/static")
	router.GET("/media/products/:filename", serveProductImage(config.ImageStore, config.Logger))
	registerRoutes(router, config.Database, config.Metrics, config.ImageStore, config.LogReader, config.SecurityKey)
	api.RegisterRoutes(router, config.Database)

	csrfMiddleware := csrf.Protect(
		config.CSRFKey,
		csrf.Secure(config.SessionSecure),
		csrf.HttpOnly(true),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.Path("/"),
		csrf.FieldName("_csrf"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.ErrorHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			config.Logger.Warn("csrf validation failed", "reason", csrf.FailureReason(request))
			http.Error(writer, "Forbidden", http.StatusForbidden)
		})),
	)
	handler := csrfMiddleware(router)
	if config.Environment != "production" {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			handler.ServeHTTP(writer, csrf.PlaintextHTTPRequest(request))
		}), nil
	}
	return handler, nil
}

func registerRoutes(router *gin.Engine, database *gorm.DB, appMetrics *metrics.Metrics, imageStore *uploads.ImageStore, logReader *logging.Reader, securityKey []byte) {
	health := handlers.NewHealthHandler(database, appMetrics)
	router.GET("/health/live", health.Live)
	router.GET("/health/ready", health.Ready)
	router.GET("/healthz", health.Live)
	router.GET("/readyz", health.Ready)

	shop := handlers.NewShopHandler(database)
	requireAuth := middleware.RequireAuth(database)
	optionalAuth := middleware.OptionalAuth(database)
	router.GET("/", optionalAuth, shop.Home)
	router.GET("/products", optionalAuth, shop.ListProducts)
	router.GET("/products/:id", optionalAuth, shop.ProductDetail)
	mailService := services.NewMailServiceFromEnv()
	securityService := services.NewAccountSecurityService(database, securityKey, mailService)
	listService := services.NewProductListService(database)
	engagementService := services.NewProductEngagementService(database)
	engagement := handlers.NewProductEngagementHandler(engagementService, listService)
	customer := router.Group("")
	customer.Use(requireAuth, middleware.RequireRoles(models.RoleCustomer))
	customer.POST("/cart/add/:id", shop.AddToCart)
	customer.GET("/cart", shop.ViewCart)
	customer.POST("/cart/items/:id", shop.UpdateCartItem)
	customer.POST("/cart/items/:id/remove", shop.RemoveCartItem)
	customer.POST("/checkout", shop.Checkout)
	customer.GET("/account/orders", shop.ListOrders)
	customer.GET("/account/orders/:id", shop.OrderDetail)
	customer.GET("/account/purchases", shop.ListOrders)
	account := handlers.NewAccountHandler(database, securityService)
	securityLimiter := middleware.NewLoginRateLimiter(20, time.Minute)
	customer.GET("/account", account.Show)
	customer.GET("/account/profile", account.Show)
	customer.POST("/account/profile", account.UpdateProfile)
	customer.POST("/account/password", securityLimiter.Middleware(), account.ChangePassword)
	customer.POST("/account/email", securityLimiter.Middleware(), account.RequestEmailChange)
	customer.POST("/account/email/confirm", securityLimiter.Middleware(), account.ConfirmEmailChange)
	customer.GET("/account/two-factor/setup", account.ShowTwoFactorSetup)
	customer.POST("/account/two-factor", securityLimiter.Middleware(), account.BeginTwoFactor)
	customer.POST("/account/two-factor/confirm", securityLimiter.Middleware(), account.ConfirmTwoFactor)
	customer.POST("/account/two-factor/disable", securityLimiter.Middleware(), account.DisableTwoFactor)
	customer.POST("/account/recovery-codes", securityLimiter.Middleware(), account.RegenerateRecoveryCodes)
	customer.POST("/account/delete", securityLimiter.Middleware(), account.DeleteAccount)
	customer.GET("/account/lists", account.ListProductLists)
	customer.POST("/account/lists", account.CreateProductList)
	customer.GET("/account/lists/:id", account.ShowProductList)
	customer.POST("/account/lists/:id/products", account.AddProductToList)
	customer.POST("/account/lists/:id/products/:productID/remove", account.RemoveProductFromList)
	customer.POST("/account/lists/:id/delete", account.DeleteProductList)
	customer.POST("/products/:id/favorite", engagement.AddFavorite)
	customer.DELETE("/products/:id/favorite", engagement.RemoveFavorite)
	customer.POST("/products/:id/lists", engagement.AddToList)
	customer.POST("/products/:id/reviews", engagement.CreateReview)
	customer.PUT("/reviews/:id", engagement.UpdateReview)
	customer.DELETE("/reviews/:id", engagement.DeleteReview)

	auth := handlers.NewAuthHandler(database, securityService)
	router.GET("/login", auth.ShowLogin)
	loginLimiter := middleware.NewLoginRateLimiter(10, time.Minute)
	router.POST("/login", loginLimiter.Middleware(), auth.Login)
	twoFactorLimiter := middleware.NewLoginRateLimiter(8, time.Minute)
	router.GET("/two-factor-challenge", func(c *gin.Context) { c.Redirect(http.StatusPermanentRedirect, "/auth/two-factor-challenge") })
	router.GET("/auth/two-factor-challenge", auth.ShowTwoFactorChallenge)
	router.POST("/auth/two-factor-challenge", twoFactorLimiter.Middleware(), auth.VerifyTwoFactorChallenge)
	router.POST("/logout", requireAuth, auth.Logout)

	admin := handlers.NewAdminHandler(database, logReader)
	adminGroup := router.Group("/admin")
	adminGroup.Use(requireAuth, middleware.RequireRoles(models.RoleAdmin))
	adminGroup.GET("/dashboard", admin.Dashboard)
	adminGroup.GET("/logs", admin.Logs)
	adminGroup.GET("/users", admin.ListUsers)
	adminGroup.POST("/users", admin.CreateUser)
	adminGroup.GET("/orders", admin.ListOrders)
	adminGroup.GET("/categories", admin.ListCategories)
	adminGroup.POST("/categories", admin.CreateCategory)
	adminGroup.POST("/categories/:id/delete", admin.DeleteCategory)

	employee := handlers.NewEmployeeHandler(database, imageStore)
	employeeGroup := router.Group("/employee")
	employeeGroup.Use(requireAuth, middleware.RequireRoles(models.RoleAdmin, models.RoleEmployee))
	employeeGroup.GET("/dashboard", employee.Dashboard)
	employeeGroup.GET("/products", employee.ListProducts)
	employeeGroup.POST("/products", employee.CreateProduct)
	employeeGroup.POST("/products/:id", employee.UpdateProduct)
	employeeGroup.POST("/products/:id/image", employee.UpdateProductImage)
	employeeGroup.POST("/products/:id/deactivate", employee.DeactivateProduct)
	employeeGroup.POST("/products/:id/stock", employee.UpdateStock)
	employeeGroup.GET("/orders", employee.ListOrders)
	employeeGroup.POST("/orders/:id/status", employee.UpdateOrderStatus)
}

func serveProductImage(imageStore *uploads.ImageStore, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		file, err := imageStore.Open(filename)
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			logger.Warn("product image unavailable", "image_id", filename)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("Content-Type", uploads.ContentType(filename))
		http.ServeContent(c.Writer, c.Request, filename, info.ModTime(), file)
	}
}

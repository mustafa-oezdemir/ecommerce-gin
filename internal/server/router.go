package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
	shippingapi "github.com/mustafa-oezdemir/ecommerce-gin/internal/shipping"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
	"gorm.io/gorm"
)

type RouterConfig struct {
	Environment                   string
	PublicURLHost                 string
	TrustedProxies                []string
	SessionSecret                 string
	SessionSecure                 bool
	CSRFKey                       []byte
	SecurityKey                   []byte
	WebhookSecret                 string
	Database                      *gorm.DB
	Metrics                       *metrics.Metrics
	Logger                        *slog.Logger
	ImageStore                    *uploads.ImageStore
	ProfileImageStore             *uploads.ImageStore
	LogReader                     *logging.Reader
	ShippingClient                shippingapi.Client
	ShippingCallbackToken         string
	ShippingPreviousCallbackToken string
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
	if config.ProfileImageStore == nil {
		return nil, errors.New("server: profile image store is required")
	}
	if config.LogReader == nil {
		return nil, errors.New("server: application log reader is required")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	router := gin.New()
	router.MaxMultipartMemory = max(config.ImageStore.MaxBytes(), config.ProfileImageStore.MaxBytes())
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
	staticFiles, err := web.StaticFS()
	if err != nil {
		return nil, fmt.Errorf("load static assets: %w", err)
	}
	router.StaticFS("/static", http.FS(staticFiles))
	router.GET("/media/products/:filename", serveProductImage(config.ImageStore, config.Logger))
	router.GET("/media/profiles/:filename", serveStoredImage(config.ProfileImageStore))
	registerRoutes(router, config.Database, config.Metrics, config.ImageStore, config.ProfileImageStore, config.LogReader, config.SecurityKey, config.Environment, config.WebhookSecret, config.ShippingClient, config.ShippingCallbackToken, config.ShippingPreviousCallbackToken)
	api.RegisterRoutes(router, config.Database)

	protectOptions := []csrf.Option{
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
	}
	csrfMiddleware := csrf.Protect(config.CSRFKey, protectOptions...)
	handler := csrfMiddleware(router)
	csrfAwareHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && (strings.HasPrefix(request.URL.Path, "/webhooks/payments/") || request.URL.Path == "/api/v1/internal/shipping/events") {
			request = csrf.UnsafeSkipCheck(request)
		}
		handler.ServeHTTP(writer, request)
	})
	if config.Environment != "production" {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			csrfAwareHandler.ServeHTTP(writer, csrf.PlaintextHTTPRequest(request))
		}), nil
	}
	return csrfAwareHandler, nil
}

func registerRoutes(router *gin.Engine, database *gorm.DB, appMetrics *metrics.Metrics, imageStore, profileImageStore *uploads.ImageStore, logReader *logging.Reader, securityKey []byte, environment, webhookSecret string, shippingClient shippingapi.Client, shippingCallbackTokens ...string) {
	health := handlers.NewHealthHandler(database, appMetrics)
	router.GET("/health/live", health.Live)
	router.GET("/health/ready", health.Ready)
	router.GET("/health", health.Live)
	router.GET("/ready", health.Ready)
	router.GET("/healthz", health.Live)
	router.GET("/readyz", health.Ready)

	mailService := services.NewMailServiceFromEnv()
	shippingService := services.NewShippingService(database, shippingClient, mailService)
	shippingHandler := handlers.NewShippingHandler(shippingService, shippingCallbackTokens...)
	shop := handlers.NewShopHandler(database, shippingService)
	checkout := handlers.NewCheckoutHandler(database, environment, webhookSecret)
	requireAuth := middleware.RequireAuth(database)
	optionalAuth := middleware.OptionalAuth(database)
	router.GET("/", optionalAuth, shop.Home)
	router.GET("/products", optionalAuth, shop.ListProducts)
	router.GET("/products/:id", optionalAuth, shop.ProductDetail)
	securityService := services.NewAccountSecurityService(database, securityKey, mailService)
	listService := services.NewProductListService(database)
	engagementService := services.NewProductEngagementService(database)
	engagement := handlers.NewProductEngagementHandler(engagementService, listService)
	customer := router.Group("")
	customer.Use(requireAuth, middleware.RequireRoles(models.RoleCustomer), middleware.LoadUnreadNotificationCount(database))
	customer.POST("/cart/add/:id", shop.AddToCart)
	customer.GET("/cart", shop.ViewCart)
	customer.POST("/cart/items/:id", shop.UpdateCartItem)
	customer.POST("/cart/items/:id/remove", shop.RemoveCartItem)
	customer.GET("/checkout", checkout.Show)
	customer.POST("/checkout", checkout.Place)
	customer.GET("/checkout/success", checkout.Success)
	customer.GET("/account/addresses", checkout.ListAddresses)
	customer.POST("/account/addresses", checkout.CreateAddress)
	customer.POST("/account/addresses/:id", checkout.UpdateAddress)
	customer.POST("/account/addresses/:id/delete", checkout.DeleteAddress)
	customer.GET("/account/orders", shop.ListOrders)
	customer.GET("/account/orders/:id", shop.OrderDetail)
	customer.POST("/account/orders/:id/confirm-delivery", shop.ConfirmDelivery)
	customer.POST("/account/orders/:id/return", shop.RequestReturn)
	customer.POST("/account/orders/:id/cancel", shop.CancelOrder)
	customer.GET("/account/purchases", shop.ListOrders)
	notifications := handlers.NewNotificationHandler(database)
	customer.GET("/account/notifications", notifications.List)
	customer.POST("/account/notifications/:id/read", notifications.MarkRead)
	customer.POST("/account/notifications/read-all", notifications.MarkAllRead)
	account := handlers.NewAccountHandler(database, securityService, profileImageStore)
	securityLimiter := middleware.NewLoginRateLimiter(20, time.Minute)
	accountGroup := router.Group("/account")
	accountGroup.Use(requireAuth)
	accountGroup.GET("", account.Show)
	accountGroup.GET("/profile", account.Show)
	accountGroup.POST("/profile", account.UpdateProfile)
	accountGroup.POST("/profile/image", account.UpdateProfileImage)
	accountGroup.POST("/profile/image/delete", account.DeleteProfileImage)
	accountGroup.POST("/password", securityLimiter.Middleware(), account.ChangePassword)
	accountGroup.POST("/email", securityLimiter.Middleware(), account.RequestEmailChange)
	accountGroup.POST("/email/confirm", securityLimiter.Middleware(), account.ConfirmEmailChange)
	accountGroup.GET("/two-factor", account.ShowTwoFactor)
	accountGroup.GET("/two-factor/setup", account.ShowTwoFactorSetup)
	accountGroup.POST("/two-factor", securityLimiter.Middleware(), account.BeginTwoFactor)
	accountGroup.POST("/two-factor/confirm", securityLimiter.Middleware(), account.ConfirmTwoFactor)
	accountGroup.POST("/two-factor/disable", securityLimiter.Middleware(), account.DisableTwoFactor)
	accountGroup.POST("/recovery-codes", securityLimiter.Middleware(), account.RegenerateRecoveryCodes)
	accountGroup.POST("/delete", securityLimiter.Middleware(), account.DeleteAccount)
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
	router.POST("/webhooks/payments/:provider", checkout.Webhook)
	router.POST("/api/v1/internal/shipping/events", shippingHandler.Callback)

	admin := handlers.NewAdminHandler(database, logReader)
	adminGroup := router.Group("/admin")
	adminGroup.Use(requireAuth, middleware.RequireRoles(models.RoleAdmin))
	adminGroup.GET("/dashboard", admin.Dashboard)
	adminGroup.GET("/logs", admin.Logs)
	adminGroup.GET("/users", admin.ListUsers)
	adminGroup.POST("/users", admin.CreateUser)
	adminGroup.POST("/users/:id", admin.UpdateUser)
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
	employeeGroup.POST("/products/:id/images", employee.UpdateProductImage)
	employeeGroup.POST("/products/:id/images/:imageID/cover", employee.SetProductCoverImage)
	employeeGroup.POST("/products/:id/images/:imageID/delete", employee.DeleteProductImage)
	employeeGroup.POST("/products/:id/deactivate", employee.DeactivateProduct)
	employeeGroup.POST("/products/:id/activate", employee.ActivateProduct)
	employeeGroup.POST("/products/:id/stock", employee.UpdateStock)
	employeeGroup.GET("/orders", employee.ListOrders)
	employeeGroup.POST("/orders/:id/status", employee.UpdateOrderStatus)
	employeeGroup.POST("/orders/:id/shipment", shippingHandler.Handover)
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

func serveStoredImage(imageStore *uploads.ImageStore) gin.HandlerFunc {
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
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("Content-Type", uploads.ContentType(filename))
		c.Header("X-Content-Type-Options", "nosniff")
		http.ServeContent(c.Writer, c.Request, filename, info.ModTime(), file)
	}
}

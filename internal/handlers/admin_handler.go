package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/logging"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AdminHandler struct {
	database  *gorm.DB
	logReader *logging.Reader
}

func NewAdminHandler(database *gorm.DB, logReader *logging.Reader) *AdminHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	if logReader == nil {
		panic("handlers: application log reader is required")
	}
	return &AdminHandler{database: database, logReader: logReader}
}

func (h *AdminHandler) Dashboard(c *gin.Context) {
	database := h.database.WithContext(c.Request.Context())
	var customers, employees, products, pendingOrders, lowStock, totalOrders int64
	var revenue struct{ Total int64 }
	queries := []func() error{
		func() error {
			return database.Model(&models.User{}).Where("role = ?", models.RoleCustomer).Count(&customers).Error
		},
		func() error {
			return database.Model(&models.User{}).Where("role = ?", models.RoleEmployee).Count(&employees).Error
		},
		func() error { return database.Model(&models.Product{}).Count(&products).Error },
		func() error {
			return applyLowStockProducts(database.Model(&models.Product{})).Count(&lowStock).Error
		},
		func() error {
			return applyPendingOrders(database.Model(&models.Order{})).Count(&pendingOrders).Error
		},
		func() error { return database.Model(&models.Order{}).Count(&totalOrders).Error },
		func() error {
			return database.Model(&models.Order{}).Select("COALESCE(SUM(total_cents), 0) AS total").Where("status <> ?", models.OrderStatusCancelled).Scan(&revenue).Error
		},
	}
	for _, query := range queries {
		if err := query(); err != nil {
			slog.Error("admin dashboard database query failed", "error", err)
			c.String(http.StatusInternalServerError, "Could not load dashboard")
			return
		}
	}
	c.HTML(http.StatusOK, "admin_dashboard.tmpl", viewData(c, gin.H{"Customers": customers, "Employees": employees, "Products": products, "PendingOrders": pendingOrders, "LowStock": lowStock, "TotalOrders": totalOrders, "RevenueCents": revenue.Total}))
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	h.renderUsers(c, http.StatusOK, "")
}

func (h *AdminHandler) renderUsers(c *gin.Context, status int, errorMessage string) {
	search := strings.TrimSpace(c.Query("q"))
	if searchRunes := []rune(search); len(searchRunes) > 100 {
		search = string(searchRunes[:100])
	}
	selectedRole := strings.ToLower(strings.TrimSpace(c.DefaultQuery("role", "all")))
	if !slices.Contains([]string{"all", string(models.RoleAdmin), string(models.RoleEmployee), string(models.RoleCustomer)}, selectedRole) {
		selectedRole = "all"
	}

	var users []models.User
	query := h.database.WithContext(c.Request.Context()).Order("created_at DESC")
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("(name LIKE ? OR email LIKE ?)", like, like)
	}
	if selectedRole != "all" {
		query = query.Where("role = ?", selectedRole)
	}
	if err := query.Find(&users).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load users")
		return
	}
	data := gin.H{"Users": users, "Search": search, "SelectedRole": selectedRole}
	if errorMessage != "" {
		data["Error"] = errorMessage
	}
	switch c.Query("status") {
	case "created":
		data["Success"] = "The user was created successfully."
	case "updated":
		data["Success"] = "The user was updated successfully."
	case "deleted":
		data["Success"] = "The user was deleted successfully."
	}
	c.HTML(status, "admin_users.tmpl", viewData(c, data))
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
		h.renderUsers(c, http.StatusBadRequest, "Enter a valid name, email address, password, and role.")
		return
	}
	name, email := strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Email))
	if utf8.RuneCountInString(name) < 2 || email == "" {
		h.renderUsers(c, http.StatusBadRequest, "Enter a valid name and email address.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not create user")
		return
	}
	user := models.User{Name: name, Email: email, Password: string(hash), Role: models.Role(req.Role)}
	if err := h.database.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			h.renderUsers(c, http.StatusConflict, "That email address is already in use.")
			return
		}
		slog.ErrorContext(c.Request.Context(), "admin user creation failed", "error", err)
		h.renderUsers(c, http.StatusInternalServerError, "The user could not be created. Please try again.")
		return
	}
	slog.InfoContext(c.Request.Context(), "admin user created", "target_user_id", user.ID, "role", user.Role)
	c.Redirect(http.StatusSeeOther, "/admin/users?status=created")
}

func (h *AdminHandler) UpdateUser(c *gin.Context) {
	currentUser, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.UserIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.UpdateAdminUserRequest
	if err := c.ShouldBind(&req); err != nil {
		h.renderUsers(c, http.StatusBadRequest, "Enter a valid name, email address, role, and a password of at least 12 characters when changing it.")
		return
	}

	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	role := models.Role(req.Role)
	if utf8.RuneCountInString(name) < 2 || email == "" {
		h.renderUsers(c, http.StatusBadRequest, "Enter a valid name and email address.")
		return
	}
	if currentUser.ID == uri.ID && role != models.RoleAdmin {
		h.renderUsers(c, http.StatusConflict, "You cannot remove your own administrator access.")
		return
	}

	database := h.database.WithContext(c.Request.Context())
	var user models.User
	if err := database.First(&user, uri.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		slog.ErrorContext(c.Request.Context(), "admin user lookup failed", "target_user_id", uri.ID, "error", err)
		h.renderUsers(c, http.StatusInternalServerError, "The user could not be loaded. Please try again.")
		return
	}

	updates := map[string]any{"name": name, "email": email, "role": role}
	passwordChanged := req.Password != ""
	if passwordChanged {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "admin password hashing failed", "target_user_id", user.ID, "error", err)
			h.renderUsers(c, http.StatusInternalServerError, "The user could not be updated. Please try again.")
			return
		}
		updates["password"] = string(hash)
		updates["security_version"] = gorm.Expr("security_version + 1")
	}
	if err := database.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			h.renderUsers(c, http.StatusConflict, "That email address is already in use.")
			return
		}
		slog.ErrorContext(c.Request.Context(), "admin user update failed", "target_user_id", user.ID, "error", err)
		h.renderUsers(c, http.StatusInternalServerError, "The user could not be updated. Please try again.")
		return
	}
	if passwordChanged && currentUser.ID == user.ID {
		var securityVersion uint64
		if err := database.Model(&models.User{}).Where("id = ?", user.ID).Pluck("security_version", &securityVersion).Error; err == nil {
			setSessionSecurityVersion(c, securityVersion)
		}
	}

	slog.InfoContext(c.Request.Context(), "admin user updated", "administrator_id", currentUser.ID, "target_user_id", user.ID, "role", role, "password_changed", passwordChanged)
	c.Redirect(http.StatusSeeOther, "/admin/users?status=updated")
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	currentUser, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.UserIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if currentUser.ID == uri.ID {
		h.renderUsers(c, http.StatusConflict, "You cannot delete your own administrator account.")
		return
	}

	database := h.database.WithContext(c.Request.Context())
	var user models.User
	if err := database.First(&user, uri.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		slog.ErrorContext(c.Request.Context(), "admin user lookup for deletion failed", "target_user_id", uri.ID, "error", err)
		h.renderUsers(c, http.StatusInternalServerError, "The user could not be loaded. Please try again.")
		return
	}
	if err := database.Delete(&user).Error; err != nil {
		slog.ErrorContext(c.Request.Context(), "admin user deletion failed", "target_user_id", user.ID, "error", err)
		h.renderUsers(c, http.StatusInternalServerError, "The user could not be deleted. Please try again.")
		return
	}

	slog.InfoContext(c.Request.Context(), "admin user deleted", "administrator_id", currentUser.ID, "target_user_id", user.ID, "role", user.Role)
	c.Redirect(http.StatusSeeOther, "/admin/users?status=deleted")
}

func (h *AdminHandler) ListOrders(c *gin.Context) {
	const pageSize = 20
	database := h.database.WithContext(c.Request.Context())
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
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	var orders []models.Order
	query := database.Model(&models.Order{}).Joins("JOIN users ON users.id = orders.user_id")
	if userSearch != "" {
		like := "%" + userSearch + "%"
		query = query.Where("(users.name LIKE ? OR users.email LIKE ?)", like, like)
	}
	if selectedStatus == models.OrderStatusPending {
		query = applyPendingOrders(query)
	} else if selectedStatus != "" {
		query = query.Where("orders.status = ?", selectedStatus)
	}
	var totalOrders int64
	if err := query.Count(&totalOrders).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load orders")
		return
	}
	totalPages := int((totalOrders + pageSize - 1) / pageSize)
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	if err := query.Preload("Items").Preload("User").Order(orderBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(&orders).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load orders")
		return
	}
	paginationQuery := url.Values{
		"user":   []string{userSearch},
		"status": []string{string(selectedStatus)},
		"sort":   []string{selectedSort},
	}.Encode()

	c.HTML(http.StatusOK, "admin_orders.tmpl", viewData(c, gin.H{
		"Orders":          orders,
		"Statuses":        managementOrderStatuses,
		"UserSearch":      userSearch,
		"SelectedStatus":  string(selectedStatus),
		"SelectedSort":    selectedSort,
		"PendingFilter":   selectedStatus == models.OrderStatusPending,
		"DashboardURL":    "/admin/dashboard",
		"Page":            page,
		"PageSize":        pageSize,
		"TotalOrders":     totalOrders,
		"TotalPages":      totalPages,
		"PageNumbers":     paginationWindow(page, totalPages),
		"PaginationQuery": paginationQuery,
	}))
}

func paginationWindow(current, total int) []int {
	if total <= 0 {
		return nil
	}
	start := max(1, current-2)
	end := min(total, start+4)
	start = max(1, end-4)
	pages := make([]int, 0, end-start+1)
	for page := start; page <= end; page++ {
		pages = append(pages, page)
	}
	return pages
}

func (h *AdminHandler) ListCategories(c *gin.Context) {
	h.renderCategories(c, http.StatusOK, "")
}

func (h *AdminHandler) renderCategories(c *gin.Context, status int, errorMessage string) {
	var categories []models.Category
	if err := h.database.WithContext(c.Request.Context()).Order("name ASC").Find(&categories).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load categories")
		return
	}
	data := gin.H{"Categories": categories}
	if errorMessage != "" {
		data["Error"] = errorMessage
	}
	if category := categoryFromQuery(categories, c.Query("view")); category != nil {
		data["ViewCategory"] = category
	} else if category := categoryFromQuery(categories, c.Query("edit")); category != nil {
		data["EditCategory"] = category
	} else if category := categoryFromQuery(categories, c.Query("delete")); category != nil {
		data["DeleteCategory"] = category
	}
	switch c.Query("status") {
	case "created":
		data["Success"] = "The category was created successfully."
	case "updated":
		data["Success"] = "The category was updated successfully."
	case "deleted":
		data["Success"] = "The category was deleted successfully."
	}
	c.HTML(status, "admin_categories.tmpl", viewData(c, data))
}

func categoryFromQuery(categories []models.Category, value string) *models.Category {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || id == 0 {
		return nil
	}
	for index := range categories {
		if categories[index].ID == uint(id) {
			return &categories[index]
		}
	}
	return nil
}

func (h *AdminHandler) CreateCategory(c *gin.Context) {
	var req validation.CreateCategoryRequest
	if err := c.ShouldBind(&req); err != nil {
		h.renderCategories(c, http.StatusBadRequest, "Enter a valid category name and description.")
		return
	}
	category := models.Category{Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description)}
	if category.Name == "" {
		h.renderCategories(c, http.StatusBadRequest, "Enter a valid category name and description.")
		return
	}
	if err := h.database.WithContext(c.Request.Context()).Create(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			h.renderCategories(c, http.StatusConflict, "A category with that name already exists.")
			return
		}
		h.renderCategories(c, http.StatusInternalServerError, "The category could not be created. Please try again.")
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/categories?status=created")
}

func (h *AdminHandler) UpdateCategory(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var req validation.CreateCategoryRequest
	if err := c.ShouldBind(&req); err != nil {
		h.renderCategories(c, http.StatusBadRequest, "Enter a valid category name and description.")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		h.renderCategories(c, http.StatusBadRequest, "Enter a valid category name and description.")
		return
	}
	result := h.database.WithContext(c.Request.Context()).Model(&models.Category{}).Where("id = ?", uri.ID).Updates(map[string]any{
		"name":        name,
		"description": strings.TrimSpace(req.Description),
	})
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			h.renderCategories(c, http.StatusConflict, "A category with that name already exists.")
			return
		}
		h.renderCategories(c, http.StatusInternalServerError, "The category could not be updated. Please try again.")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/categories?status=updated")
}

func (h *AdminHandler) DeleteCategory(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	result := h.database.WithContext(c.Request.Context()).Delete(&models.Category{}, uri.ID)
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not delete category")
		return
	}
	if result.RowsAffected != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/categories?status=deleted")
}

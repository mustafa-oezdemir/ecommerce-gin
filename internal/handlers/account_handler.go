package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

type AccountHandler struct{ productListService *services.ProductListService }

func NewAccountHandler() *AccountHandler {
	return &AccountHandler{productListService: services.NewProductListService()}
}

func (h *AccountHandler) Show(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.HTML(http.StatusOK, "account.tmpl", viewData(c, gin.H{"User": user}))
}

func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.UpdateProfileRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid profile data")
		return
	}
	name, email := strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Email))
	if name == "" || email == "" {
		c.String(http.StatusBadRequest, "Invalid profile data")
		return
	}
	if err := db.DB.Model(user).Updates(map[string]any{"name": name, "email": email}).Error; err != nil {
		c.String(http.StatusConflict, "Could not update profile")
		return
	}
	c.Redirect(http.StatusFound, "/account")
}

func (h *AccountHandler) ChangePassword(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.ChangePasswordRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid password data")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		c.String(http.StatusBadRequest, "Could not change password")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not change password")
		return
	}
	if err := db.DB.Model(user).Update("password", string(hash)).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not change password")
		return
	}
	go services.NewMailServiceFromEnv().SendPasswordChanged(*user)
	c.Redirect(http.StatusFound, "/account")
}

func (h *AccountHandler) ListProductLists(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	lists, err := h.productListService.List(user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load lists")
		return
	}
	c.HTML(http.StatusOK, "product_lists.tmpl", viewData(c, gin.H{"Lists": lists}))
}

func (h *AccountHandler) CreateProductList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.CreateProductListRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid list name")
		return
	}
	if err := h.productListService.Create(user.ID, req.Name); err != nil {
		c.String(http.StatusBadRequest, "Could not create list")
		return
	}
	c.Redirect(http.StatusFound, "/account/lists")
}

func (h *AccountHandler) ShowProductList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	list, err := h.productListService.Get(user.ID, uri.ID)
	if err != nil {
		if errors.Is(err, services.ErrProductListNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusInternalServerError, "Could not load list")
		return
	}
	products, err := h.productListService.AvailableProducts()
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	c.HTML(http.StatusOK, "product_list_detail.tmpl", viewData(c, gin.H{"List": list, "Products": products}))
}

func (h *AccountHandler) AddProductToList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListIDURI
	var req validation.AddProductToListRequest
	if err := c.ShouldBindUri(&uri); err != nil || c.ShouldBind(&req) != nil {
		c.String(http.StatusBadRequest, "Invalid product")
		return
	}
	if err := h.productListService.AddProduct(user.ID, uri.ID, req.ProductID); err != nil {
		if errors.Is(err, services.ErrProductListNotFound) || errors.Is(err, services.ErrProductNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusBadRequest, "Could not add product to list")
		return
	}
	c.Redirect(http.StatusFound, "/account/lists/"+c.Param("id"))
}

func (h *AccountHandler) RemoveProductFromList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListProductURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.productListService.RemoveProduct(user.ID, uri.ListID, uri.ProductID); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, "/account/lists/"+c.Param("id"))
}

func (h *AccountHandler) DeleteProductList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.productListService.Delete(user.ID, uri.ID); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, "/account/lists")
}

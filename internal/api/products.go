package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

var zeroPricePattern = regexp.MustCompile(`^0+(?:[.,]0{1,2})?$`)

type productStore interface {
	ListActive(context.Context, repositories.ProductQuery) ([]models.Product, int64, error)
	GetActiveByID(context.Context, uint) (*models.Product, error)
}

type ProductHandler struct {
	store productStore
}

type productListQuery struct {
	Limit      int    `form:"limit,default=20" binding:"gte=1,lte=100"`
	Offset     int    `form:"offset,default=0" binding:"gte=0,lte=1000000"`
	CategoryID uint   `form:"category" binding:"omitempty,gt=0"`
	MinPrice   string `form:"min_price" binding:"max=32"`
	MaxPrice   string `form:"max_price" binding:"max=32"`
	Search     string `form:"q" binding:"max=100"`
	Sort       string `form:"sort,default=created_at" binding:"oneof=created_at price name"`
	Order      string `form:"order,default=desc" binding:"oneof=asc desc"`
}

type productResource struct {
	ID          uint              `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	ImageURL    string            `json:"image_url,omitempty"`
	PriceCents  int64             `json:"price_cents"`
	Stock       int               `json:"stock"`
	Category    *categoryResource `json:"category,omitempty"`
}

type categoryResource struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func NewProductHandler(database *gorm.DB) *ProductHandler {
	return newProductHandler(repositories.NewProductRepository(database))
}

func newProductHandler(store productStore) *ProductHandler {
	return &ProductHandler{store: store}
}

func (handler *ProductHandler) List(c *gin.Context) {
	var request productListQuery
	if err := c.ShouldBindQuery(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_QUERY", "Query parameters are invalid")
		return
	}
	minPrice, err := parseOptionalPrice(request.MinPrice)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_MIN_PRICE", "Minimum price is invalid")
		return
	}
	maxPrice, err := parseOptionalPrice(request.MaxPrice)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_MAX_PRICE", "Maximum price is invalid")
		return
	}
	if minPrice != nil && maxPrice != nil && *minPrice > *maxPrice {
		respondError(c, http.StatusBadRequest, "INVALID_PRICE_RANGE", "Minimum price cannot exceed maximum price")
		return
	}

	products, total, err := handler.store.ListActive(c.Request.Context(), repositories.ProductQuery{
		Limit:         request.Limit,
		Offset:        request.Offset,
		CategoryID:    request.CategoryID,
		MinPriceCents: minPrice,
		MaxPriceCents: maxPrice,
		Search:        strings.TrimSpace(request.Search),
		Sort:          request.Sort,
		Order:         request.Order,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "DATABASE_ERROR", "Products could not be loaded")
		return
	}
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}

	resources := make([]productResource, 0, len(products))
	for index := range products {
		resources = append(resources, newProductResource(&products[index]))
	}
	totalPages := int((total + int64(request.Limit) - 1) / int64(request.Limit))
	respondOK(c, http.StatusOK, resources, &pagination{
		Limit:      request.Limit,
		Offset:     request.Offset,
		Page:       request.Offset/request.Limit + 1,
		Total:      total,
		TotalPages: totalPages,
	})
}

func (handler *ProductHandler) Get(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		respondError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product was not found")
		return
	}
	product, err := handler.store.GetActiveByID(c.Request.Context(), uri.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product was not found")
		} else {
			respondError(c, http.StatusInternalServerError, "DATABASE_ERROR", "Product could not be loaded")
		}
		return
	}
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	respondOK(c, http.StatusOK, newProductResource(product), nil)
}

func (handler *ProductHandler) Options(c *gin.Context) {
	c.Header("Allow", "GET, HEAD, OPTIONS")
	c.Status(http.StatusNoContent)
}

func parseOptionalPrice(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if zeroPricePattern.MatchString(value) {
		zero := int64(0)
		return &zero, nil
	}
	cents, err := validation.ParseCents(value)
	if err != nil {
		return nil, err
	}
	return &cents, nil
}

func newProductResource(product *models.Product) productResource {
	resource := productResource{
		ID:          product.ID,
		Name:        product.Name,
		Description: product.Description,
		PriceCents:  product.PriceCents,
		Stock:       product.Stock,
	}
	if product.ImageFilename != "" {
		resource.ImageURL = "/media/products/" + product.ImageFilename
	}
	if product.Category != nil {
		resource.Category = &categoryResource{ID: product.Category.ID, Name: product.Category.Name}
	}
	return resource
}

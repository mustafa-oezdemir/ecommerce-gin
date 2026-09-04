package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"gorm.io/gorm"
)

type productStoreStub struct {
	list func(context.Context, repositories.ProductQuery) ([]models.Product, int64, error)
	get  func(context.Context, uint) (*models.Product, error)
}

func (store productStoreStub) ListActive(ctx context.Context, filter repositories.ProductQuery) ([]models.Product, int64, error) {
	return store.list(ctx, filter)
}

func (store productStoreStub) GetActiveByID(ctx context.Context, productID uint) (*models.Product, error) {
	return store.get(ctx, productID)
}

func TestProductListReturnsFilteredPaginatedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	categoryID := uint(3)
	handler := newProductHandler(productStoreStub{
		list: func(_ context.Context, filter repositories.ProductQuery) ([]models.Product, int64, error) {
			if filter.Limit != 10 || filter.Offset != 20 || filter.CategoryID != categoryID || filter.Search != "phone" || filter.Sort != "price" || filter.Order != "asc" {
				t.Fatalf("unexpected filter: %#v", filter)
			}
			if filter.MinPriceCents == nil || *filter.MinPriceCents != 1050 || filter.MaxPriceCents == nil || *filter.MaxPriceCents != 9999 {
				t.Fatalf("unexpected price filter: %#v", filter)
			}
			return []models.Product{{Model: gorm.Model{ID: 5}, Name: "Phone", PriceCents: 2500, Stock: 4}}, 24, nil
		},
		get: func(context.Context, uint) (*models.Product, error) { return nil, nil },
	})

	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/products?limit=10&offset=20&category=3&min_price=10.50&max_price=99.99&q=%20phone%20&sort=price&order=asc", nil)
	context, _ := gin.CreateTestContext(responseRecorder)
	context.Request = request
	handler.List(context)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    []productResource
		Meta    pagination
	}
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || len(body.Data) != 1 || body.Data[0].ID != 5 || body.Meta.Page != 3 || body.Meta.Total != 24 || body.Meta.TotalPages != 3 {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestProductResourceIncludesOrderedGalleryAndLegacyCover(t *testing.T) {
	product := models.Product{
		Model:         gorm.Model{ID: 5},
		Name:          "Phone",
		ImageFilename: "cover.jpg",
		Images: []models.ProductImage{
			{ID: 1, Filename: "side.png", Position: 0},
			{ID: 2, Filename: "cover.jpg", Position: 1},
		},
	}
	resource := newProductResource(&product)
	if resource.ImageURL != "/media/products/cover.jpg" {
		t.Fatalf("unexpected cover URL %q", resource.ImageURL)
	}
	if len(resource.ImageURLs) != 2 || resource.ImageURLs[0] != "/media/products/cover.jpg" || resource.ImageURLs[1] != "/media/products/side.png" {
		t.Fatalf("unexpected gallery URLs: %#v", resource.ImageURLs)
	}
}

func TestProductListRejectsInvalidRangeWithoutQueryingStore(t *testing.T) {
	called := false
	handler := newProductHandler(productStoreStub{
		list: func(context.Context, repositories.ProductQuery) ([]models.Product, int64, error) {
			called = true
			return nil, 0, nil
		},
		get: func(context.Context, uint) (*models.Product, error) { return nil, nil },
	})

	responseRecorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(responseRecorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/products?min_price=20&max_price=10", nil)
	handler.List(context)

	if responseRecorder.Code != http.StatusBadRequest || called {
		t.Fatalf("expected validation failure without store call, status=%d called=%t", responseRecorder.Code, called)
	}
	assertErrorCode(t, responseRecorder, "INVALID_PRICE_RANGE")
}

func TestProductDetailReturnsStructuredNotFound(t *testing.T) {
	handler := newProductHandler(productStoreStub{
		list: func(context.Context, repositories.ProductQuery) ([]models.Product, int64, error) { return nil, 0, nil },
		get:  func(context.Context, uint) (*models.Product, error) { return nil, gorm.ErrRecordNotFound },
	})

	responseRecorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(responseRecorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/products/99", nil)
	context.Params = gin.Params{{Key: "id", Value: "99"}}
	handler.Get(context)

	if responseRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", responseRecorder.Code)
	}
	assertErrorCode(t, responseRecorder, "PRODUCT_NOT_FOUND")
}

func TestProductListReturnsStructuredDatabaseError(t *testing.T) {
	handler := newProductHandler(productStoreStub{
		list: func(context.Context, repositories.ProductQuery) ([]models.Product, int64, error) {
			return nil, 0, errors.New("database unavailable")
		},
		get: func(context.Context, uint) (*models.Product, error) { return nil, nil },
	})

	responseRecorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(responseRecorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/products", nil)
	handler.List(context)

	if responseRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", responseRecorder.Code)
	}
	assertErrorCode(t, responseRecorder, "DATABASE_ERROR")
}

func assertErrorCode(t *testing.T, responseRecorder *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Success || body.Error.Code != expected {
		t.Fatalf("expected error %q, got %#v", expected, body)
	}
}

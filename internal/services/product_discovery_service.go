package services

import (
	"context"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
)

const productPageSize = 12

type ProductDiscoveryStore interface {
	ListActive(context.Context, repositories.ProductQuery) ([]models.Product, int64, error)
	FilterOptions(context.Context) (repositories.ProductFilterOptions, error)
}

type ProductDiscoveryRequest struct {
	Page          int
	CategoryIDs   []uint
	BrandIDs      []uint
	MinPriceCents *int64
	MaxPriceCents *int64
	Search        string
	Colors        []string
	ClothingSizes []string
	ShoeSizes     []string
	MinimumRating int
	InStock       bool
	Attributes    map[string][]string
	Sort          string
}

type ProductDiscoveryResult struct {
	Products   []models.Product
	Options    repositories.ProductFilterOptions
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

type ProductDiscoveryService struct{ store ProductDiscoveryStore }

func NewProductDiscoveryService(store ProductDiscoveryStore) *ProductDiscoveryService {
	return &ProductDiscoveryService{store: store}
}

func (service *ProductDiscoveryService) Search(ctx context.Context, request ProductDiscoveryRequest) (ProductDiscoveryResult, error) {
	request = normalizeProductDiscoveryRequest(request)
	sortName, orderName := productDiscoverySort(request.Sort)
	products, total, err := service.store.ListActive(ctx, repositories.ProductQuery{
		Limit: productPageSize, Offset: (request.Page - 1) * productPageSize,
		CategoryIDs: request.CategoryIDs, BrandIDs: request.BrandIDs,
		MinPriceCents: request.MinPriceCents, MaxPriceCents: request.MaxPriceCents,
		Search: request.Search, Colors: request.Colors, ClothingSizes: request.ClothingSizes,
		ShoeSizes: request.ShoeSizes, MinimumRating: request.MinimumRating, InStock: request.InStock,
		Attributes: request.Attributes, Sort: sortName, Order: orderName,
	})
	if err != nil {
		return ProductDiscoveryResult{}, err
	}
	options, err := service.store.FilterOptions(ctx)
	if err != nil {
		return ProductDiscoveryResult{}, err
	}
	totalPages := int((total + productPageSize - 1) / productPageSize)
	return ProductDiscoveryResult{Products: products, Options: options, Total: total, Page: request.Page, PageSize: productPageSize, TotalPages: totalPages}, nil
}

func normalizeProductDiscoveryRequest(request ProductDiscoveryRequest) ProductDiscoveryRequest {
	if request.Page < 1 {
		request.Page = 1
	}
	request.Search = strings.TrimSpace(request.Search)
	if request.MinimumRating < 1 || request.MinimumRating > 10 {
		request.MinimumRating = 0
	}
	if request.MinPriceCents != nil && *request.MinPriceCents < 0 {
		request.MinPriceCents = nil
	}
	if request.MaxPriceCents != nil && *request.MaxPriceCents < 0 {
		request.MaxPriceCents = nil
	}
	if request.MinPriceCents != nil && request.MaxPriceCents != nil && *request.MinPriceCents > *request.MaxPriceCents {
		request.MinPriceCents, request.MaxPriceCents = request.MaxPriceCents, request.MinPriceCents
	}
	return request
}

func productDiscoverySort(value string) (string, string) {
	switch value {
	case "price_asc":
		return "price", "asc"
	case "price_desc":
		return "price", "desc"
	case "rating_asc":
		return "rating", "asc"
	case "rating_desc":
		return "rating", "desc"
	case "reviews_asc":
		return "reviews", "asc"
	case "reviews_desc":
		return "reviews", "desc"
	case "oldest":
		return "created_at", "asc"
	case "name_asc":
		return "name", "asc"
	default:
		return "created_at", "desc"
	}
}

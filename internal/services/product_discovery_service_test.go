package services

import (
	"context"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
)

type productDiscoveryStoreStub struct {
	query repositories.ProductQuery
}

func (store *productDiscoveryStoreStub) ListActive(_ context.Context, query repositories.ProductQuery) ([]models.Product, int64, error) {
	store.query = query
	return []models.Product{{Name: "Shoe"}}, 25, nil
}

func (*productDiscoveryStoreStub) FilterOptions(context.Context) (repositories.ProductFilterOptions, error) {
	return repositories.ProductFilterOptions{}, nil
}

func TestProductDiscoverySearchNormalizesAndPaginates(t *testing.T) {
	store := &productDiscoveryStoreStub{}
	service := NewProductDiscoveryService(store)
	minimum := int64(15000)
	maximum := int64(5000)
	result, err := service.Search(context.Background(), ProductDiscoveryRequest{
		Page: 2, Search: "  running shoe  ", MinPriceCents: &minimum, MaxPriceCents: &maximum,
		MinimumRating: 8, Sort: "price_asc", Colors: []string{"Black"}, ShoeSizes: []string{"42"},
	})
	if err != nil {
		t.Fatalf("search products: %v", err)
	}
	if store.query.Offset != 12 || store.query.Limit != 12 || store.query.Search != "running shoe" {
		t.Fatalf("unexpected normalized query: %#v", store.query)
	}
	if store.query.MinPriceCents == nil || *store.query.MinPriceCents != 5000 || store.query.MaxPriceCents == nil || *store.query.MaxPriceCents != 15000 {
		t.Fatalf("expected reversed price range to be normalized: %#v", store.query)
	}
	if store.query.Sort != "price" || store.query.Order != "asc" || result.TotalPages != 3 || result.Page != 2 {
		t.Fatalf("unexpected result/query: %#v %#v", result, store.query)
	}
}

func TestProductDiscoveryRejectsUnlistedSort(t *testing.T) {
	store := &productDiscoveryStoreStub{}
	service := NewProductDiscoveryService(store)
	if _, err := service.Search(context.Background(), ProductDiscoveryRequest{Sort: "price_cents; DROP TABLE products"}); err != nil {
		t.Fatalf("search products: %v", err)
	}
	if store.query.Sort != "created_at" || store.query.Order != "desc" {
		t.Fatalf("unsafe sort was not replaced: %#v", store.query)
	}
}

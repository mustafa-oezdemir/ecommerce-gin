package handlers

import (
	"net/url"
	"strings"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"gorm.io/gorm"
)

func TestParseProductFiltersBuildsDatabaseRequest(t *testing.T) {
	values, _ := url.ParseQuery("q=runner&category=2&category=3&brand=4&color=Black&clothing_size=L&shoe_size=42&min_price=50&max_price=150&rating=8&in_stock=1&attribute.Material=Leather&sort=price_asc&page=2")
	filters, err := parseProductFilters(values)
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	request := filters.discoveryRequest()
	if request.Page != 2 || request.Search != "runner" || len(request.CategoryIDs) != 2 || len(request.BrandIDs) != 1 || request.MinimumRating != 8 || !request.InStock || request.Sort != "price_asc" {
		t.Fatalf("unexpected request: %#v", request)
	}
	if request.MinPriceCents == nil || *request.MinPriceCents != 5000 || request.MaxPriceCents == nil || *request.MaxPriceCents != 15000 {
		t.Fatalf("unexpected prices: %#v", request)
	}
	if got := request.Attributes["Material"]; len(got) != 1 || got[0] != "Leather" {
		t.Fatalf("unexpected attributes: %#v", request.Attributes)
	}
}

func TestProductFilterChipRemovesOnlySelectedValueAndKeepsSort(t *testing.T) {
	values, _ := url.ParseQuery("brand=4&brand=7&color=Black&sort=rating_desc&page=3&min_price=&rating=")
	filters, err := parseProductFilters(values)
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	result := services.ProductDiscoveryResult{
		Options: repositories.ProductFilterOptions{Brands: []models.Brand{
			{Model: gorm.Model{ID: 4}, Name: "Nike"},
			{Model: gorm.Model{ID: 7}, Name: "Adidas"},
		}},
		Page: 3, TotalPages: 4,
	}
	view := newProductFilterView(values, filters, result)
	if view.ActiveCount != 3 || len(view.Chips) != 3 {
		t.Fatalf("unexpected chips: %#v", view.Chips)
	}
	if view.Chips[0].Label != "Nike" || !strings.Contains(view.Chips[0].RemoveURL, "brand=7") || strings.Contains(view.Chips[0].RemoveURL, "brand=4") || !strings.Contains(view.Chips[0].RemoveURL, "sort=rating_desc") {
		t.Fatalf("unexpected removal URL: %#v", view.Chips[0])
	}
	if strings.Contains(view.Chips[0].RemoveURL, "min_price=") || strings.Contains(view.Chips[0].RemoveURL, "rating=") {
		t.Fatalf("empty controls leaked into chip URL: %q", view.Chips[0].RemoveURL)
	}
	if strings.Contains(view.NextURL, "page=3") || !strings.Contains(view.NextURL, "page=4") {
		t.Fatalf("unexpected next URL: %q", view.NextURL)
	}
}

func TestParseProductFiltersRejectsInvalidPrice(t *testing.T) {
	if _, err := parseProductFilters(url.Values{"min_price": []string{"free"}}); err == nil {
		t.Fatal("expected invalid price to be rejected")
	}
}

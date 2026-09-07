package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/repositories"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
)

const maxProductFilterValues = 30

type productFilters struct {
	Page          int
	Search        string
	CategoryIDs   []uint
	BrandIDs      []uint
	MinPrice      string
	MaxPrice      string
	MinPriceCents *int64
	MaxPriceCents *int64
	Colors        []string
	ClothingSizes []string
	ShoeSizes     []string
	Rating        int
	InStock       bool
	Attributes    map[string][]string
	Sort          string
}

type productFilterChip struct {
	Label     string
	RemoveURL string
}

type productFilterView struct {
	Search             string
	MinPrice           string
	MaxPrice           string
	Rating             int
	InStock            bool
	Sort               string
	SelectedCategories map[uint]bool
	SelectedBrands     map[uint]bool
	SelectedColors     map[string]bool
	SelectedClothing   map[string]bool
	SelectedShoes      map[string]bool
	SelectedAttributes map[string]bool
	Options            repositories.ProductFilterOptions
	Chips              []productFilterChip
	ActiveCount        int
	PreviousURL        string
	NextURL            string
	PageURLs           []productPageLink
}

type productPageLink struct {
	Number  int
	URL     string
	Current bool
}

func parseProductFilters(values url.Values) (productFilters, error) {
	filters := productFilters{
		Page:          parseBoundedPositiveInt(values.Get("page"), 1, 10000),
		Search:        strings.TrimSpace(values.Get("q")),
		MinPrice:      strings.TrimSpace(values.Get("min_price")),
		MaxPrice:      strings.TrimSpace(values.Get("max_price")),
		Colors:        cleanFilterValues(values["color"]),
		ClothingSizes: cleanFilterValues(values["clothing_size"]),
		ShoeSizes:     cleanFilterValues(values["shoe_size"]),
		Rating:        parseBoundedPositiveInt(values.Get("rating"), 0, 10),
		InStock:       values.Get("in_stock") == "1" || strings.EqualFold(values.Get("in_stock"), "true"),
		Sort:          normalizeProductSort(values.Get("sort")),
		Attributes:    map[string][]string{},
	}
	if len(filters.Search) > 100 {
		return productFilters{}, errors.New("search is too long")
	}
	filters.CategoryIDs = parseUintValues(values["category"])
	filters.BrandIDs = parseUintValues(values["brand"])
	var err error
	if filters.MinPriceCents, err = parseFilterPrice(filters.MinPrice); err != nil {
		return productFilters{}, err
	}
	if filters.MaxPriceCents, err = parseFilterPrice(filters.MaxPrice); err != nil {
		return productFilters{}, err
	}
	for key, candidates := range values {
		if !strings.HasPrefix(key, "attribute.") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(key, "attribute."))
		if name == "" || len(name) > 80 {
			continue
		}
		if selected := cleanFilterValues(candidates); len(selected) > 0 {
			filters.Attributes[name] = selected
		}
	}
	return filters, nil
}

func (filters productFilters) discoveryRequest() services.ProductDiscoveryRequest {
	return services.ProductDiscoveryRequest{
		Page: filters.Page, CategoryIDs: filters.CategoryIDs, BrandIDs: filters.BrandIDs,
		MinPriceCents: filters.MinPriceCents, MaxPriceCents: filters.MaxPriceCents,
		Search: filters.Search, Colors: filters.Colors, ClothingSizes: filters.ClothingSizes,
		ShoeSizes: filters.ShoeSizes, MinimumRating: filters.Rating, InStock: filters.InStock,
		Attributes: filters.Attributes, Sort: filters.Sort,
	}
}

func newProductFilterView(values url.Values, filters productFilters, result services.ProductDiscoveryResult) productFilterView {
	view := productFilterView{
		Search: filters.Search, MinPrice: filters.MinPrice, MaxPrice: filters.MaxPrice,
		Rating: filters.Rating, InStock: filters.InStock, Sort: filters.Sort, Options: result.Options,
		SelectedCategories: uintSet(filters.CategoryIDs), SelectedBrands: uintSet(filters.BrandIDs),
		SelectedColors: stringSet(filters.Colors), SelectedClothing: stringSet(filters.ClothingSizes),
		SelectedShoes: stringSet(filters.ShoeSizes), SelectedAttributes: map[string]bool{},
	}
	for name, selected := range filters.Attributes {
		for _, value := range selected {
			view.SelectedAttributes[name+"\x00"+value] = true
		}
	}
	base := cloneQuery(values)
	base.Del("page")
	removeEmptyQueryValues(base)
	if filters.Search != "" {
		view.addChip("Search: "+filters.Search, base, "q", filters.Search)
	}
	categoryNames := make(map[uint]string, len(result.Options.Categories))
	for _, category := range result.Options.Categories {
		categoryNames[category.ID] = category.Name
	}
	for _, id := range filters.CategoryIDs {
		view.addChip(valueLabel(categoryNames[id], "Category", id), base, "category", strconv.FormatUint(uint64(id), 10))
	}
	brandNames := make(map[uint]string, len(result.Options.Brands))
	for _, brand := range result.Options.Brands {
		brandNames[brand.ID] = brand.Name
	}
	for _, id := range filters.BrandIDs {
		view.addChip(valueLabel(brandNames[id], "Brand", id), base, "brand", strconv.FormatUint(uint64(id), 10))
	}
	for _, value := range filters.Colors {
		view.addChip(value, base, "color", value)
	}
	for _, value := range filters.ClothingSizes {
		view.addChip("Size "+value, base, "clothing_size", value)
	}
	for _, value := range filters.ShoeSizes {
		view.addChip("EU "+value, base, "shoe_size", value)
	}
	if filters.MinPrice != "" || filters.MaxPrice != "" {
		label := fmt.Sprintf("€%s–€%s", fallback(filters.MinPrice, "0"), fallback(filters.MaxPrice, "∞"))
		remove := cloneQuery(base)
		remove.Del("min_price")
		remove.Del("max_price")
		view.Chips = append(view.Chips, productFilterChip{Label: label, RemoveURL: queryURL(remove)})
	}
	if filters.Rating > 0 {
		view.addChip(fmt.Sprintf("Rating %d+", filters.Rating), base, "rating", strconv.Itoa(filters.Rating))
	}
	if filters.InStock {
		view.addChip("In stock", base, "in_stock", values.Get("in_stock"))
	}
	keys := make([]string, 0, len(filters.Attributes))
	for key := range filters.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range filters.Attributes[key] {
			view.addChip(key+": "+value, base, "attribute."+key, value)
		}
	}
	view.ActiveCount = len(view.Chips)
	if result.Page > 1 {
		previous := cloneQuery(base)
		previous.Set("page", strconv.Itoa(result.Page-1))
		view.PreviousURL = queryURL(previous)
	}
	if result.Page < result.TotalPages {
		next := cloneQuery(base)
		next.Set("page", strconv.Itoa(result.Page+1))
		view.NextURL = queryURL(next)
	}
	for page := 1; page <= result.TotalPages; page++ {
		pageValues := cloneQuery(base)
		pageValues.Set("page", strconv.Itoa(page))
		view.PageURLs = append(view.PageURLs, productPageLink{Number: page, URL: queryURL(pageValues), Current: page == result.Page})
	}
	return view
}

func (view *productFilterView) addChip(label string, base url.Values, key, value string) {
	remove := cloneQuery(base)
	remaining := make([]string, 0, len(remove[key]))
	removed := false
	for _, candidate := range remove[key] {
		if !removed && candidate == value {
			removed = true
			continue
		}
		remaining = append(remaining, candidate)
	}
	remove.Del(key)
	for _, candidate := range remaining {
		remove.Add(key, candidate)
	}
	view.Chips = append(view.Chips, productFilterChip{Label: label, RemoveURL: queryURL(remove)})
}

func parseFilterPrice(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	if value == "0" || value == "0.0" || value == "0.00" || value == "0,0" || value == "0,00" {
		zero := int64(0)
		return &zero, nil
	}
	cents, err := validation.ParseCents(value)
	if err != nil {
		return nil, err
	}
	return &cents, nil
}

func parseUintValues(values []string) []uint {
	result := make([]uint, 0, len(values))
	seen := map[uint]bool{}
	for _, value := range values {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err == nil && parsed > 0 && !seen[uint(parsed)] && len(result) < maxProductFilterValues {
			seen[uint(parsed)] = true
			result = append(result, uint(parsed))
		}
	}
	return result
}

func cleanFilterValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && len(value) <= 120 && !seen[value] && len(result) < maxProductFilterValues {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func parseBoundedPositiveInt(value string, fallbackValue, maximum int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallbackValue
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}
func normalizeProductSort(value string) string {
	switch value {
	case "price_asc", "price_desc", "rating_asc", "rating_desc", "reviews_asc", "reviews_desc", "oldest", "name_asc":
		return value
	default:
		return "newest"
	}
}
func uintSet(values []uint) map[uint]bool {
	result := map[uint]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
func cloneQuery(values url.Values) url.Values {
	cloned := url.Values{}
	for key, list := range values {
		cloned[key] = append([]string(nil), list...)
	}
	return cloned
}
func removeEmptyQueryValues(values url.Values) {
	for key, candidates := range values {
		kept := candidates[:0]
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate) != "" {
				kept = append(kept, candidate)
			}
		}
		if len(kept) == 0 {
			values.Del(key)
		} else {
			values[key] = kept
		}
	}
}
func queryURL(values url.Values) string {
	encoded := values.Encode()
	if encoded == "" {
		return "/"
	}
	return "/?" + encoded
}
func fallback(value, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}
func valueLabel(value, prefix string, id uint) string {
	if value != "" {
		return value
	}
	return fmt.Sprintf("%s #%d", prefix, id)
}

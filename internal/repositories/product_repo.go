package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type ProductQuery struct {
	Limit         int
	Offset        int
	CategoryID    uint
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
	Order         string
}

type AttributeOption struct {
	Name   string
	Values []string
}

type ProductFilterOptions struct {
	Categories    []models.Category
	Brands        []models.Brand
	Colors        []string
	ClothingSizes []string
	ShoeSizes     []string
	Attributes    []AttributeOption
}

type ProductRepository struct {
	database *gorm.DB
}

func NewProductRepository(database *gorm.DB) *ProductRepository {
	return &ProductRepository{database: database}
}

func (repository *ProductRepository) ListActive(ctx context.Context, filter ProductQuery) ([]models.Product, int64, error) {
	query := repository.filteredQuery(ctx, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := productSortClause(filter.Sort, filter.Order)
	var products []models.Product
	err := query.Select("products.*").Preload("Category").Preload("Brand").Preload("Images", productImageOrder).
		Order(order).Limit(filter.Limit).Offset(filter.Offset).Find(&products).Error
	return products, total, err
}

func (repository *ProductRepository) FilterOptions(ctx context.Context) (ProductFilterOptions, error) {
	options := ProductFilterOptions{}
	database := repository.database.WithContext(ctx)
	if err := database.Order("name ASC").Find(&options.Categories).Error; err != nil {
		return options, err
	}
	if err := database.Order("name ASC").Find(&options.Brands).Error; err != nil {
		return options, err
	}
	for column, destination := range map[string]*[]string{
		"color":         &options.Colors,
		"clothing_size": &options.ClothingSizes,
		"shoe_size":     &options.ShoeSizes,
	} {
		if err := database.Model(&models.ProductVariant{}).Distinct(column).
			Where("active = ? AND "+column+" <> ''", true).Order(column+" ASC").Pluck(column, destination).Error; err != nil {
			return options, err
		}
	}

	type attributeRow struct{ Name, Value string }
	var rows []attributeRow
	if err := database.Model(&models.ProductVariantAttribute{}).Distinct("name", "value").
		Order("name ASC, value ASC").Find(&rows).Error; err != nil {
		return options, err
	}
	for _, row := range rows {
		if len(options.Attributes) == 0 || options.Attributes[len(options.Attributes)-1].Name != row.Name {
			options.Attributes = append(options.Attributes, AttributeOption{Name: row.Name})
		}
		last := len(options.Attributes) - 1
		options.Attributes[last].Values = append(options.Attributes[last].Values, row.Value)
	}
	return options, nil
}

func (repository *ProductRepository) filteredQuery(ctx context.Context, filter ProductQuery) *gorm.DB {
	reviewStats := repository.database.Table("product_reviews").
		Select("product_id, AVG(rating) AS average_rating, COUNT(*) AS review_count").Group("product_id")
	query := repository.database.WithContext(ctx).Model(&models.Product{}).
		Joins("LEFT JOIN brands ON brands.id = products.brand_id AND brands.deleted_at IS NULL").
		Joins("LEFT JOIN (?) AS review_stats ON review_stats.product_id = products.id", reviewStats).
		Where("products.active = ?", true)

	categoryIDs := filter.CategoryIDs
	if len(categoryIDs) == 0 && filter.CategoryID > 0 {
		categoryIDs = []uint{filter.CategoryID}
	}
	if len(categoryIDs) > 0 {
		query = query.Where("products.category_id IN ?", categoryIDs)
	}
	if len(filter.BrandIDs) > 0 {
		query = query.Where("products.brand_id IN ?", filter.BrandIDs)
	}
	if filter.MinPriceCents != nil {
		query = query.Where("products.price_cents >= ?", *filter.MinPriceCents)
	}
	if filter.MaxPriceCents != nil {
		query = query.Where("products.price_cents <= ?", *filter.MaxPriceCents)
	}
	if filter.Search != "" {
		like := "%" + strings.ToLower(filter.Search) + "%"
		query = query.Where("(LOWER(products.name) LIKE ? OR LOWER(products.description) LIKE ? OR LOWER(COALESCE(brands.name, '')) LIKE ? OR EXISTS (SELECT 1 FROM product_variants search_variant WHERE search_variant.product_id = products.id AND search_variant.deleted_at IS NULL AND LOWER(search_variant.sku) LIKE ?))", like, like, like, like)
	}
	if filter.MinimumRating > 0 {
		query = query.Where("COALESCE(review_stats.average_rating, 0) >= ?", filter.MinimumRating)
	}
	if filter.InStock {
		query = query.Where("((NOT EXISTS (SELECT 1 FROM product_variants stock_variant WHERE stock_variant.product_id = products.id AND stock_variant.deleted_at IS NULL) AND products.stock > 0) OR EXISTS (SELECT 1 FROM product_variants stock_variant WHERE stock_variant.product_id = products.id AND stock_variant.deleted_at IS NULL AND stock_variant.active = ? AND stock_variant.stock > 0))", true)
	}
	return addVariantFilters(query, filter)
}

func addVariantFilters(query *gorm.DB, filter ProductQuery) *gorm.DB {
	if len(filter.Colors) == 0 && len(filter.ClothingSizes) == 0 && len(filter.ShoeSizes) == 0 && len(filter.Attributes) == 0 {
		return query
	}
	conditions := []string{"variant.product_id = products.id", "variant.deleted_at IS NULL", "variant.active = ?"}
	arguments := []any{true}
	if len(filter.Colors) > 0 {
		conditions = append(conditions, "variant.color IN ?")
		arguments = append(arguments, filter.Colors)
	}
	if len(filter.ClothingSizes) > 0 {
		conditions = append(conditions, "variant.clothing_size IN ?")
		arguments = append(arguments, filter.ClothingSizes)
	}
	if len(filter.ShoeSizes) > 0 {
		conditions = append(conditions, "variant.shoe_size IN ?")
		arguments = append(arguments, filter.ShoeSizes)
	}
	keys := make([]string, 0, len(filter.Attributes))
	for key := range filter.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		values := filter.Attributes[key]
		if len(values) == 0 {
			continue
		}
		alias := fmt.Sprintf("attribute_%d", index)
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM product_variant_attributes %s WHERE %s.product_variant_id = variant.id AND %s.deleted_at IS NULL AND %s.name = ? AND %s.value IN ?)", alias, alias, alias, alias, alias))
		arguments = append(arguments, key, values)
	}
	statement := "EXISTS (SELECT 1 FROM product_variants variant WHERE " + strings.Join(conditions, " AND ") + ")"
	return query.Where(statement, arguments...)
}

func productSortClause(sortName, orderName string) string {
	direction := "DESC"
	if strings.EqualFold(orderName, "asc") {
		direction = "ASC"
	}
	column := "products.created_at"
	switch sortName {
	case "name":
		column = "products.name"
	case "price":
		column = "products.price_cents"
	case "rating":
		column = "COALESCE(review_stats.average_rating, 0)"
	case "reviews":
		column = "COALESCE(review_stats.review_count, 0)"
	}
	return column + " " + direction + ", products.id DESC"
}

func (repository *ProductRepository) GetActiveByID(ctx context.Context, productID uint) (*models.Product, error) {
	var product models.Product
	err := repository.database.WithContext(ctx).Preload("Category").Preload("Brand").Preload("Images", productImageOrder).
		Preload("Variants", "active = ?", true).Preload("Variants.Attributes").
		Where("id = ? AND active = ?", productID, true).First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func productImageOrder(database *gorm.DB) *gorm.DB {
	return database.Order("position ASC, id ASC")
}

package repositories

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestVariantFiltersUseOneCorrelatedVariant(t *testing.T) {
	sqlDatabase, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDatabase, SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	query := database.Model(&models.Product{})
	query = addVariantFilters(query, ProductQuery{Colors: []string{"Black"}, ShoeSizes: []string{"42"}})
	statement := query.Find(&[]models.Product{}).Statement.SQL.String()

	if strings.Count(statement, "EXISTS (SELECT 1 FROM product_variants variant") != 1 {
		t.Fatalf("expected one same-variant subquery, got %s", statement)
	}
	if !regexp.MustCompile(`variant\.color IN \(\?\).*variant\.shoe_size IN \(\?\)`).MatchString(statement) {
		t.Fatalf("color and shoe size must be constrained on the same variant: %s", statement)
	}
}

func TestProductSortClauseUsesWhitelist(t *testing.T) {
	if got := productSortClause("price_cents; DROP TABLE products", "asc"); got != "products.created_at ASC, products.id DESC" {
		t.Fatalf("unexpected fallback sort: %q", got)
	}
	if got := productSortClause("rating", "desc"); got != "COALESCE(review_stats.average_rating, 0) DESC, products.id DESC" {
		t.Fatalf("unexpected rating sort: %q", got)
	}
}

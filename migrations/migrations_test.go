package migrations

import (
	"strings"
	"testing"
)

func TestSplitStatementsHandlesMixedLineEndings(t *testing.T) {
	statements := splitStatements("CREATE TABLE one (id INT);\r\nCREATE TABLE two (id INT);\n")
	if len(statements) != 3 || statements[0] != "CREATE TABLE one (id INT)" || statements[1] != "CREATE TABLE two (id INT)" {
		t.Fatalf("unexpected statements: %#v", statements)
	}
}

func TestProductDiscoveryMigrationPreservesVariantCombinations(t *testing.T) {
	for _, expected := range []string{"CREATE TABLE IF NOT EXISTS brands", "CREATE TABLE IF NOT EXISTS product_variants", "CREATE TABLE IF NOT EXISTS product_variant_attributes", "idx_products_price_cents"} {
		if !strings.Contains(productDiscoverySchema, expected) {
			t.Fatalf("product discovery migration is missing %q", expected)
		}
	}
}

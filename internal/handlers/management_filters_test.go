package handlers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

func TestLowStockScopeUsesDashboardBoundaries(t *testing.T) {
	database, _ := newMockHandlerDatabase(t)
	query := applyLowStockProducts(database.Session(&gorm.Session{DryRun: true}).Model(&models.Product{})).Find(&[]models.Product{})
	var count int64
	countQuery := applyLowStockProducts(database.Session(&gorm.Session{DryRun: true}).Model(&models.Product{})).Count(&count)

	if sql := query.Statement.SQL.String(); !strings.Contains(sql, "products.active = ? AND products.stock BETWEEN ? AND ?") {
		t.Fatalf("low-stock query does not contain the shared active/range rule: %s", sql)
	}
	if got, want := query.Statement.Vars, []any{true, lowStockMin, lowStockMax}; !reflect.DeepEqual(got, want) {
		t.Fatalf("low-stock query arguments = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(countQuery.Statement.Vars, query.Statement.Vars) {
		t.Fatalf("dashboard count arguments %#v differ from list arguments %#v", countQuery.Statement.Vars, query.Statement.Vars)
	}
	for _, test := range []struct {
		stock int
		want  bool
	}{{0, false}, {1, true}, {5, true}, {6, false}} {
		got := test.stock >= lowStockMin && test.stock <= lowStockMax
		if got != test.want {
			t.Errorf("stock %d low-stock classification = %t, want %t", test.stock, got, test.want)
		}
	}
}

func TestPendingScopeUsesOnlyPendingStatus(t *testing.T) {
	database, _ := newMockHandlerDatabase(t)
	query := applyPendingOrders(database.Session(&gorm.Session{DryRun: true}).Model(&models.Order{})).Find(&[]models.Order{})
	var count int64
	countQuery := applyPendingOrders(database.Session(&gorm.Session{DryRun: true}).Model(&models.Order{})).Count(&count)

	if sql := query.Statement.SQL.String(); !strings.Contains(sql, "orders.status = ?") {
		t.Fatalf("pending query does not contain the shared status rule: %s", sql)
	}
	if got, want := query.Statement.Vars, []any{models.OrderStatusPending}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pending query arguments = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(countQuery.Statement.Vars, query.Statement.Vars) {
		t.Fatalf("dashboard count arguments %#v differ from list arguments %#v", countQuery.Statement.Vars, query.Statement.Vars)
	}
	if normalizeManagementOrderStatus("processing") == models.OrderStatusPending {
		t.Fatal("non-pending order was classified as pending")
	}
}

func TestManagementFiltersNormalizeInvalidParametersSafely(t *testing.T) {
	if got := normalizeStockStatus(" LOW "); got != stockStatusLow {
		t.Fatalf("normalized stock status = %q", got)
	}
	if got := normalizeStockStatus("unknown"); got != "" {
		t.Fatalf("invalid stock status = %q, want empty", got)
	}
	if got := normalizeManagementOrderStatus(" PENDING "); got != models.OrderStatusPending {
		t.Fatalf("normalized order status = %q", got)
	}
	if got := normalizeManagementOrderStatus("unknown"); got != "" {
		t.Fatalf("invalid order status = %q, want empty", got)
	}
}

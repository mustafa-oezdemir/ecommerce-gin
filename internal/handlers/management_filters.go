package handlers

import (
	"slices"
	"strings"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

const (
	stockStatusLow = "low"
	lowStockMin    = 1
	lowStockMax    = 5
)

var managementOrderStatuses = []models.OrderStatus{
	models.OrderStatusPending,
	models.OrderStatusPaid,
	models.OrderStatusPreparing,
	models.OrderStatusReadyForShipping,
	models.OrderStatusProcessing,
	models.OrderStatusShipped,
	models.OrderStatusCompleted,
	models.OrderStatusCancelled,
}

func normalizeStockStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == stockStatusLow {
		return value
	}
	return ""
}

func normalizeManagementOrderStatus(value string) models.OrderStatus {
	status := models.OrderStatus(strings.ToLower(strings.TrimSpace(value)))
	if slices.Contains(managementOrderStatuses, status) {
		return status
	}
	return ""
}

// lowStockProducts applies the single inventory rule used by dashboard counts
// and filtered management lists. Stock zero remains a separate out-of-stock KPI.
func applyLowStockProducts(database *gorm.DB) *gorm.DB {
	return database.Where("products.active = ? AND products.stock BETWEEN ? AND ?", true, lowStockMin, lowStockMax)
}

// pendingOrders applies the single order rule used by dashboard counts and
// filtered management lists.
func applyPendingOrders(database *gorm.DB) *gorm.DB {
	return database.Where("orders.status = ?", models.OrderStatusPending)
}

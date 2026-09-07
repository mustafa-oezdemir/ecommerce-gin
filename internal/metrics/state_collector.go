package metrics

import (
	"context"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

var orderStatuses = []models.OrderStatus{
	models.OrderStatusPending, models.OrderStatusPendingPayment, models.OrderStatusPaid,
	models.OrderStatusPaymentFailed, models.OrderStatusRefunded, models.OrderStatusPreparing,
	models.OrderStatusReadyForShipping, models.OrderStatusProcessing, models.OrderStatusShipped,
	models.OrderStatusCompleted, models.OrderStatusCancelled,
}

type StateCollector struct {
	database         *gorm.DB
	ordersCurrent    *prometheus.Desc
	inventoryCurrent *prometheus.Desc
	returnsCurrent   *prometheus.Desc
	ordersToday      *prometheus.Desc
	revenueToday     *prometheus.Desc
}

func NewStateCollector(database *gorm.DB) *StateCollector {
	return &StateCollector{
		database:         database,
		ordersCurrent:    prometheus.NewDesc("ecommerce_orders_current", "Current orders by status.", []string{"status"}, nil),
		inventoryCurrent: prometheus.NewDesc("ecommerce_inventory_current", "Current active products by stock state.", []string{"state"}, nil),
		returnsCurrent:   prometheus.NewDesc("ecommerce_returns_current", "Current return requests by status.", []string{"status"}, nil),
		ordersToday:      prometheus.NewDesc("ecommerce_orders_today", "Orders created since the current UTC day began.", nil, nil),
		revenueToday:     prometheus.NewDesc("ecommerce_revenue_today_cents", "Non-refunded paid order value since the current UTC day began, in cents.", nil, nil),
	}
}

func (collector *StateCollector) Describe(channel chan<- *prometheus.Desc) {
	channel <- collector.ordersCurrent
	channel <- collector.inventoryCurrent
	channel <- collector.returnsCurrent
	channel <- collector.ordersToday
	channel <- collector.revenueToday
}

func (collector *StateCollector) Collect(channel chan<- prometheus.Metric) {
	if collector.database == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	orderCounts := make(map[string]int64, len(orderStatuses))
	var orderRows []struct {
		Status string
		Count  int64
	}
	if collector.database.WithContext(ctx).Model(&models.Order{}).Select("status, count(*) count").Group("status").Scan(&orderRows).Error == nil {
		for _, row := range orderRows {
			orderCounts[row.Status] = row.Count
		}
		for _, status := range orderStatuses {
			channel <- prometheus.MustNewConstMetric(collector.ordersCurrent, prometheus.GaugeValue, float64(orderCounts[string(status)]), string(status))
		}
	}

	stockQueries := map[string]string{
		"active":       "active = ?",
		"low_stock":    "active = ? AND stock BETWEEN 1 AND 5",
		"out_of_stock": "active = ? AND stock <= 0",
	}
	for state, query := range stockQueries {
		var count int64
		if collector.database.WithContext(ctx).Model(&models.Product{}).Where(query, true).Count(&count).Error == nil {
			channel <- prometheus.MustNewConstMetric(collector.inventoryCurrent, prometheus.GaugeValue, float64(count), state)
		}
	}

	var returnRows []struct {
		Status string
		Count  int64
	}
	if collector.database.WithContext(ctx).Model(&models.ReturnRequest{}).Select("status, count(*) count").Group("status").Scan(&returnRows).Error == nil {
		for _, row := range returnRows {
			channel <- prometheus.MustNewConstMetric(collector.returnsCurrent, prometheus.GaugeValue, float64(row.Count), row.Status)
		}
	}

	dayStart := time.Now().UTC().Truncate(24 * time.Hour)
	var todayCount int64
	if collector.database.WithContext(ctx).Model(&models.Order{}).Where("created_at >= ?", dayStart).Count(&todayCount).Error == nil {
		channel <- prometheus.MustNewConstMetric(collector.ordersToday, prometheus.GaugeValue, float64(todayCount))
	}
	var revenue struct{ Total int64 }
	paidStatuses := []models.OrderStatus{models.OrderStatusPaid, models.OrderStatusPreparing, models.OrderStatusReadyForShipping, models.OrderStatusProcessing, models.OrderStatusShipped, models.OrderStatusCompleted}
	if collector.database.WithContext(ctx).Model(&models.Order{}).Select("COALESCE(SUM(total_cents), 0) total").Where("created_at >= ? AND status IN ?", dayStart, paidStatuses).Scan(&revenue).Error == nil {
		channel <- prometheus.MustNewConstMetric(collector.revenueToday, prometheus.GaugeValue, float64(revenue.Total))
	}
}

package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

func ParseTemplates() (*template.Template, error) {
	return template.New("root").Funcs(template.FuncMap{
		"money":             formatCents,
		"priceInput":        formatPriceInput,
		"mulCents":          mulCents,
		"initials":          initials,
		"nextOrderStatuses": models.AllowedOrderStatusTransitions,
		"orderStatusLabel":  orderStatusLabel,
		"add":               func(a, b int) int { return a + b },
		"sub":               func(a, b int) int { return a - b },
		"listRatings":       func() []int { return []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1} },
	}).ParseFS(templateFS, "templates/*.tmpl")
}

func orderStatusLabel(status models.OrderStatus) string {
	switch status {
	case models.OrderStatusPending:
		return "Pending"
	case models.OrderStatusPendingPayment:
		return "Pending payment"
	case models.OrderStatusPaid:
		return "Paid"
	case models.OrderStatusPaymentFailed:
		return "Payment failed"
	case models.OrderStatusRefunded:
		return "Refunded"
	case models.OrderStatusProcessing:
		return "Processing"
	case models.OrderStatusShipped:
		return "Shipped"
	case models.OrderStatusCompleted:
		return "Completed"
	case models.OrderStatusCancelled:
		return "Cancelled"
	default:
		return string(status)
	}
}

func formatCents(cents int64) string {
	if cents < 0 {
		return "-" + formatCents(-cents)
	}
	return fmt.Sprintf("%d,%02d €", cents/100, cents%100)
}

func formatPriceInput(cents int64) string {
	if cents < 0 {
		return "-" + formatPriceInput(-cents)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func mulCents(priceCents int64, quantity int) int64 {
	return priceCents * int64(quantity)
}

func initials(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.Fields(trimmed)
	first, _ := utf8.DecodeRuneInString(parts[0])
	if first == utf8.RuneError {
		return ""
	}
	result := string(first)
	if len(parts) > 1 {
		last, _ := utf8.DecodeRuneInString(parts[len(parts)-1])
		if last != utf8.RuneError {
			result += string(last)
		}
	}
	return strings.ToUpper(result)
}

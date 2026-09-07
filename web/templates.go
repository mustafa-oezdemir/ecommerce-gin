package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

//go:embed all:templates
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

func ParseTemplates() (*template.Template, error) {
	templates := template.New("root").Funcs(template.FuncMap{
		"money":             formatCents,
		"priceInput":        formatPriceInput,
		"mulCents":          mulCents,
		"initials":          initials,
		"nextOrderStatuses": models.AllowedOrderStatusTransitions,
		"orderStatusLabel":  orderStatusLabel,
		"statusLabel":       statusLabel,
		"add":               func(a, b int) int { return a + b },
		"sub":               func(a, b int) int { return a - b },
		"listRatings":       func() []int { return []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1} },
		"attributeSelected": func(selected map[string]bool, name, value string) bool { return selected[name+"\x00"+value] },
		"primarySKU": func(variants []models.ProductVariant) string {
			if len(variants) == 0 || variants[0].SKU == "" {
				return "—"
			}
			return variants[0].SKU
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires key/value pairs")
			}
			result := make(map[string]any, len(values)/2)
			for index := 0; index < len(values); index += 2 {
				key, ok := values[index].(string)
				if !ok {
					return nil, fmt.Errorf("dict key must be a string")
				}
				result[key] = values[index+1]
			}
			return result, nil
		},
	})
	var filenames []string
	if err := fs.WalkDir(templateFS, "templates", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".tmpl" {
			filenames = append(filenames, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk templates: %w", err)
	}
	sort.Strings(filenames)
	for _, filename := range filenames {
		contents, err := fs.ReadFile(templateFS, filename)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", filename, err)
		}
		if _, err := templates.New(filename).Parse(string(contents)); err != nil {
			return nil, fmt.Errorf("parse template %s: %w", filename, err)
		}
	}
	return templates, nil
}

func statusLabel(value any) string {
	words := strings.Fields(strings.ReplaceAll(fmt.Sprint(value), "_", " "))
	for index, word := range words {
		if word == "" {
			continue
		}
		runes := []rune(strings.ToLower(word))
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		words[index] = string(runes)
	}
	return strings.Join(words, " ")
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

package handlers

import "testing"

func TestActiveNavigationUsesStableSectionKeys(t *testing.T) {
	tests := map[string]string{
		"/":                         "products",
		"/products/7":               "products",
		"/cart":                     "cart",
		"/checkout":                 "cart",
		"/account":                  "account",
		"/account/addresses/7/edit": "account",
		"/account/orders/63":        "orders",
		"/account/notifications":    "notifications",
		"/admin/dashboard":          "dashboard",
		"/admin/users/7/edit":       "users",
		"/admin/orders/63":          "orders",
		"/admin/categories":         "categories",
		"/admin/logs":               "logs",
		"/employee/dashboard":       "dashboard",
		"/employee/products/7/edit": "products",
		"/employee/orders/63":       "orders",
	}
	for path, expected := range tests {
		if actual := activeNavigation(path); actual != expected {
			t.Errorf("activeNavigation(%q) = %q, want %q", path, actual, expected)
		}
	}
}

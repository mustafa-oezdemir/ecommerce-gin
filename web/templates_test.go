package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

func TestEmployeeOrdersShowsOnlyAllowedTransitions(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	tests := []struct {
		name      string
		status    models.OrderStatus
		want      []string
		doNotWant []string
	}{
		{
			name:      "pending",
			status:    models.OrderStatusPending,
			want:      []string{`value="processing"`, `value="cancelled"`},
			doNotWant: []string{`value="shipped"`, `value="completed"`},
		},
		{
			name:      "completed",
			status:    models.OrderStatusCompleted,
			want:      []string{"Completed", "No further actions"},
			doNotWant: []string{`action="/employee/orders/7/status"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			data := map[string]any{
				"CSRFField": template.HTML(`<input type="hidden" name="csrf">`),
				"Orders":    []models.Order{{Model: gorm.Model{ID: 7}, Status: tt.status}},
			}
			if err := templates.ExecuteTemplate(&output, "employee_orders.tmpl", data); err != nil {
				t.Fatalf("execute template: %v", err)
			}
			body := output.String()
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Errorf("output does not contain %q", want)
				}
			}
			for _, unwanted := range tt.doNotWant {
				if strings.Contains(body, unwanted) {
					t.Errorf("output unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}

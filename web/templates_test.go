package web

import (
	"bytes"
	"html/template"
	"io/fs"
	"strings"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/logging"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

func TestStaticFSContainsCSPCompatibleScripts(t *testing.T) {
	assets, err := StaticFS()
	if err != nil {
		t.Fatalf("open static filesystem: %v", err)
	}
	for _, name := range []string{"account.js", "product-list.js", "product-detail.js"} {
		contents, err := fs.ReadFile(assets, name)
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if len(contents) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestAccountTemplateUsesExternalScriptWithoutInlineHandlers(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "account.tmpl", map[string]any{
		"User":      models.User{Email: "customer@example.com"},
		"CSRFField": template.HTML(`<input type="hidden" name="_csrf">`),
	}); err != nil {
		t.Fatalf("execute account template: %v", err)
	}
	body := output.String()
	if !strings.Contains(body, `src="/static/account.js"`) {
		t.Fatal("account template is missing its external script")
	}
	if strings.Contains(body, "onsubmit=") || strings.Contains(body, "<script>") {
		t.Fatal("account template contains CSP-incompatible inline JavaScript")
	}
	if !strings.Contains(body, `action="/logout"`) || !strings.Contains(body, ">Log out</button>") {
		t.Fatal("account template is missing the logout form")
	}
}

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

func TestAdminLogsRendersStructuredEntriesAndEscapesValues(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var output bytes.Buffer
	data := map[string]any{
		"Snapshot": logging.LogSnapshot{
			FileSizeText: "1.0 KB",
			Entries: []logging.LogEntry{{
				Time:       "2026-09-04 10:00:00.000 UTC",
				Level:      "ERROR",
				LevelClass: "danger",
				Message:    `<script>alert("unsafe")</script>`,
				Attributes: []logging.LogAttribute{{Name: "route", Value: "/checkout"}},
			}},
		},
		"Level":  "all",
		"Limit":  100,
		"Search": "",
	}
	if err := templates.ExecuteTemplate(&output, "admin_logs.tmpl", data); err != nil {
		t.Fatalf("execute log template: %v", err)
	}
	body := output.String()
	if strings.Contains(body, `<script>alert`) || !strings.Contains(body, `&lt;script&gt;alert`) {
		t.Fatalf("log message was not safely escaped: %s", body)
	}
	if !strings.Contains(body, "Application Logs") || !strings.Contains(body, "/checkout") {
		t.Fatalf("log view is missing expected content: %s", body)
	}
}

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
	for _, name := range []string{"account.js", "product-list.js", "product-detail.js", "site.css"} {
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
	user := models.User{Email: "customer@example.com", Role: models.RoleCustomer}
	if err := templates.ExecuteTemplate(&output, "account.tmpl", map[string]any{
		"User":        user,
		"CurrentUser": &user,
		"CSRFField":   template.HTML(`<input type="hidden" name="_csrf">`),
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

func TestSharedShellRendersBrandAndRoleNavigation(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	tests := []struct {
		name      string
		data      map[string]any
		want      []string
		doNotWant []string
	}{
		{name: "anonymous", data: map[string]any{}, want: []string{"brand-pehli\">Pehli", "brand-one\">One", `href="/login"`}, doNotWant: []string{`class="account-dropdown"`, `action="/logout"`}},
		{name: "customer", data: map[string]any{"CurrentUser": &models.User{Name: "Ada Lovelace", Email: "ada@example.com", Role: models.RoleCustomer}, "CSRFField": template.HTML("csrf")}, want: []string{`href="/cart"`, `class="account-dropdown"`, `class="account-avatar"`, `>A</span>`, "Ada Lovelace", "ada@example.com", `href="/account"`, `href="/account/orders"`, `href="/account/lists"`, `action="/logout"`}, doNotWant: []string{`href="/login"`, `aria-hidden="true">⌄`}},
		{name: "employee", data: map[string]any{"CurrentUser": &models.User{Role: models.RoleEmployee}, "CSRFField": template.HTML("csrf")}, want: []string{`href="/employee/dashboard"`, `href="/employee/products"`, `class="account-dropdown"`, `action="/logout"`}, doNotWant: []string{`href="/login"`}},
		{name: "admin", data: map[string]any{"CurrentUser": &models.User{Role: models.RoleAdmin}, "CSRFField": template.HTML("csrf")}, want: []string{`href="/admin/dashboard"`, `href="/admin/logs"`, `class="account-dropdown"`, `action="/logout"`}, doNotWant: []string{`href="/login"`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := templates.ExecuteTemplate(&output, "site-nav", test.data); err != nil {
				t.Fatalf("execute navigation: %v", err)
			}
			for _, want := range test.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("navigation does not contain %q: %s", want, output.String())
				}
			}
			for _, unwanted := range test.doNotWant {
				if strings.Contains(output.String(), unwanted) {
					t.Errorf("navigation unexpectedly contains %q: %s", unwanted, output.String())
				}
			}
		})
	}
}

func TestAdminUsersTemplateRendersSecureEditForms(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	admin := models.User{Model: gorm.Model{ID: 1}, Name: "Admin User", Email: "admin@example.com", Role: models.RoleAdmin}
	employee := models.User{Model: gorm.Model{ID: 7}, Name: "Employee User", Email: "employee@example.com", Role: models.RoleEmployee}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "admin_users.tmpl", map[string]any{
		"CurrentUser": &admin,
		"Users":       []models.User{admin, employee},
		"CSRFField":   template.HTML(`<input type="hidden" name="_csrf" value="test">`),
		"Success":     "The user was updated successfully.",
	}); err != nil {
		t.Fatalf("execute admin users template: %v", err)
	}
	body := output.String()
	for _, want := range []string{
		`action="/admin/users/1"`,
		`action="/admin/users/7"`,
		`name="name" value="Employee User"`,
		`name="email" value="employee@example.com"`,
		`id="user-password-7" type="password" name="password"`,
		`placeholder="Leave blank to keep current password"`,
		`<option value="employee" selected>Employee</option>`,
		`You cannot remove your own administrator access.`,
		`The user was updated successfully.`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin users page does not contain %q", want)
		}
	}
	if strings.Contains(body, "onsubmit=") || strings.Contains(body, "onclick=") {
		t.Fatal("admin users template contains CSP-incompatible inline handlers")
	}
}

func TestEmployeeProductsTemplateRendersMultiImageManagement(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	employee := models.User{Model: gorm.Model{ID: 2}, Name: "Employee User", Role: models.RoleEmployee}
	product := models.Product{
		Model:         gorm.Model{ID: 7},
		Name:          "Camera",
		ImageFilename: "cover.jpg",
		Images: []models.ProductImage{
			{ID: 11, ProductID: 7, Filename: "cover.jpg"},
			{ID: 12, ProductID: 7, Filename: "side.png", Position: 1},
		},
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "employee_products.tmpl", map[string]any{
		"CurrentUser": &employee,
		"Products":    []models.Product{product},
		"CSRFField":   template.HTML("csrf"),
		"ImageMaxMB":  5,
		"ImageLimit":  8,
	}); err != nil {
		t.Fatalf("execute employee products template: %v", err)
	}
	body := output.String()
	for _, want := range []string{
		`name="images"`, `multiple`,
		`action="/employee/products/7/images"`,
		`action="/employee/products/7/images/12/cover"`,
		`action="/employee/products/7/images/11/delete"`,
		`2 / 8 images`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("employee image management does not contain %q", want)
		}
	}
}

func TestEveryPageTemplateUsesSharedShell(t *testing.T) {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		t.Fatalf("read templates: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "layout.tmpl" {
			continue
		}
		contents, err := fs.ReadFile(templateFS, "templates/"+entry.Name())
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		page := string(contents)
		for _, required := range []string{`href="/static/site.css"`, `template "site-nav"`, `class="app-main`, `template "site-footer"`} {
			if !strings.Contains(page, required) {
				t.Errorf("%s does not use shared shell marker %q", entry.Name(), required)
			}
		}
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

func TestAdminOrdersRendersUserAndStatusFilters(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	user := models.User{Model: gorm.Model{ID: 7}, Name: "Ada Lovelace", Email: "ada@example.com"}
	var output bytes.Buffer
	data := map[string]any{
		"Orders":         []models.Order{{Model: gorm.Model{ID: 11}, User: user, Status: models.OrderStatusProcessing}},
		"Statuses":       []models.OrderStatus{models.OrderStatusPending, models.OrderStatusProcessing},
		"UserSearch":     "ada@example.com",
		"SelectedStatus": "processing",
		"SelectedSort":   "total_desc",
	}
	if err := templates.ExecuteTemplate(&output, "admin_orders.tmpl", data); err != nil {
		t.Fatalf("execute admin orders template: %v", err)
	}
	body := output.String()
	for _, want := range []string{
		`action="/admin/orders"`, `name="user" value="ada@example.com"`, `name="status"`, `name="sort"`,
		`value="processing" selected`, `value="total_desc" selected`, `href="/admin/orders">Reset`,
		`Ada Lovelace`, `ada@example.com`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin orders page does not contain %q", want)
		}
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

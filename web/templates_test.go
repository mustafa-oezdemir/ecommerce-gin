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
	for _, name := range []string{"account.js", "checkout.js", "product-list.js", "product-detail.js", "site.css"} {
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
	user := models.User{FirstName: "Mustafa", LastName: "Özdemir", Email: "customer@example.com", Role: models.RoleCustomer}
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
	for _, want := range []string{
		`name="first_name" value="Mustafa"`,
		`name="last_name" value="Özdemir"`,
		`id="profile-email" type="email" value="customer@example.com" readonly`,
		`action="/account/profile/image" enctype="multipart/form-data"`,
		`accept="image/jpeg,image/png,image/webp"`,
		`class="profile-photo profile-photo-placeholder"`,
		`action="/account/password"`,
		`href="/account/two-factor"`,
		`Status: Not enabled`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("account template does not contain %q", want)
		}
	}
}

func TestAccountAndNavbarRenderStoredProfileImage(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Name: "Ada Lovelace", FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com", Role: models.RoleCustomer, ProfileImageFilename: "0123456789abcdef0123456789abcdef.png"}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "account.tmpl", map[string]any{"User": &user, "CurrentUser": &user, "CSRFField": template.HTML(`<input name="_csrf">`)}); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, expected := range []string{
		`src="/media/profiles/0123456789abcdef0123456789abcdef.png"`,
		`alt="Current profile photo"`,
		`action="/account/profile/image/delete"`,
		`class="account-avatar account-avatar-image"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("profile image template missing %q", expected)
		}
	}
	if strings.Contains(body, `<span class="profile-photo profile-photo-placeholder"`) {
		t.Fatal("stored image rendered the profile placeholder")
	}
}

func TestTwoFactorManagementTemplateShowsEnabledState(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	user := models.User{TwoFactorEnabled: true}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "two_factor.tmpl", map[string]any{
		"User":      &user,
		"CSRFField": template.HTML(`<input type="hidden" name="_csrf">`),
	}); err != nil {
		t.Fatalf("execute two-factor template: %v", err)
	}
	body := output.String()
	for _, want := range []string{`Status: Enabled`, `action="/account/recovery-codes"`, `action="/account/two-factor/disable"`, `name="current_password"`, `name="code"`} {
		if !strings.Contains(body, want) {
			t.Errorf("two-factor management template does not contain %q", want)
		}
	}
	if strings.Contains(body, `action="/account/two-factor"><`) {
		t.Fatal("enabled two-factor page unexpectedly renders setup action")
	}
}

func TestCheckoutTemplateUsesServerSummaryAndSafePaymentOptions(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Name: "Ada Lovelace", Role: models.RoleCustomer}
	address := models.UserAddress{Model: gorm.Model{ID: 4}, FirstName: "Ada", LastName: "Lovelace", Street: "Main", HouseNumber: "1", PostalCode: "12345", City: "Berlin", CountryCode: "DE", IsDefault: true}
	product := models.Product{Model: gorm.Model{ID: 8}, Name: "Go Book", PriceCents: 2000, Active: true}
	summary := struct {
		Cart          *models.Cart
		SubtotalCents int64
		ShippingCents int64
		TotalCents    int64
	}{Cart: &models.Cart{Items: []models.CartItem{{Product: product, Quantity: 2}}}, SubtotalCents: 4000, ShippingCents: 499, TotalCents: 4499}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "checkout.tmpl", map[string]any{"CurrentUser": &user, "Addresses": []models.UserAddress{address}, "Summary": summary, "IdempotencyKey": strings.Repeat("a", 64), "CSRFField": template.HTML("csrf")}); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, expected := range []string{"action=\"/checkout\"", "name=\"idempotency_key\"", "value=\"debit_card\"", "value=\"credit_card\"", "value=\"paypal\"", "value=\"klarna\"", "Go Book × 2", "44,99 €", "src=\"/static/checkout.js\""} {
		if !strings.Contains(body, expected) {
			t.Errorf("checkout template missing %q", expected)
		}
	}
	for _, forbidden := range []string{`name="card_number"`, `name="cvc"`, `name="cvv"`} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("checkout template contains PCI-sensitive field %q", forbidden)
		}
	}
}

func TestOrderDetailUsesEachActiveProductsCoverAndLink(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	order := models.Order{
		Model: gorm.Model{ID: 17},
		Items: []models.OrderItem{
			{
				ProductID:   4,
				ProductName: "Historical Camera Name",
				ProductSKU:  "CAM-4",
				Product: models.Product{
					Model:         gorm.Model{ID: 4},
					Name:          "Current Camera Name",
					ImageFilename: "cover-camera.png",
					Images: []models.ProductImage{
						{ID: 10, Filename: "secondary-camera.png", Position: 0},
						{ID: 11, Filename: "cover-camera.png", Position: 1},
					},
				},
			},
			{
				ProductID:   12,
				ProductName: "Product Without Image",
				Product:     models.Product{Model: gorm.Model{ID: 12}},
			},
			{
				ProductID:   29,
				ProductName: "Third Ordered Product",
				Product: models.Product{
					Model:         gorm.Model{ID: 29},
					ImageFilename: "cover-third-product.png",
				},
			},
			{
				ProductID:   30,
				ProductName: "Deleted Historical Product",
			},
		},
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "order_detail.tmpl", map[string]any{"Order": order}); err != nil {
		t.Fatalf("execute order detail template: %v", err)
	}
	body := output.String()
	for _, expected := range []string{
		`href="/products/4"`,
		`href="/products/12"`,
		`href="/products/29"`,
		`src="/media/products/cover-camera.png"`,
		`src="/media/products/cover-third-product.png"`,
		`alt="Historical Camera Name"`,
		`Historical Camera Name`,
		`Product Without Image`,
		`Deleted Historical Product`,
		`order-product-placeholder`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("order detail template missing %q", expected)
		}
	}
	for _, unexpected := range []string{
		`href="/products/17"`,
		`href="/products/30"`,
		`secondary-camera.png`,
		`Current Camera Name`,
	} {
		if strings.Contains(body, unexpected) {
			t.Errorf("order detail template unexpectedly contains %q", unexpected)
		}
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
		{name: "customer", data: map[string]any{"CurrentUser": &models.User{Name: "Ada Lovelace", Email: "ada@example.com", Role: models.RoleCustomer}, "CSRFField": template.HTML("csrf")}, want: []string{`href="/cart"`, `class="account-dropdown"`, `class="account-avatar"`, `>AL</span>`, "Ada Lovelace", "ada@example.com", `href="/account"`, `href="/account/orders"`, `href="/account/lists"`, `action="/logout"`}, doNotWant: []string{`href="/login"`, `aria-hidden="true">⌄`}},
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
		"CurrentUser":  &admin,
		"Users":        []models.User{admin, employee},
		"CSRFField":    template.HTML(`<input type="hidden" name="_csrf" value="test">`),
		"Success":      "The user was updated successfully.",
		"Search":       "employee@example.com",
		"SelectedRole": "employee",
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
		`id="new-user"`,
		`action="/admin/users"`,
		`name="q" value="employee@example.com"`,
		`name="role"`,
		`value="employee" selected`,
		`href="/admin/users">Reset`,
		`src="/static/admin-users.js"`,
		`action="/admin/users/7/delete"`,
		`class="delete-user-form`,
		`Delete user`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin users page does not contain %q", want)
		}
	}
	if strings.Contains(body, "onsubmit=") || strings.Contains(body, "onclick=") {
		t.Fatal("admin users template contains CSP-incompatible inline handlers")
	}
	if strings.Contains(body, `action="/admin/users/1/delete"`) {
		t.Fatal("admin users template allows the current administrator to delete itself")
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
		PriceCents:    12999,
		Active:        true,
		Images: []models.ProductImage{
			{ID: 11, ProductID: 7, Filename: "cover.jpg"},
			{ID: 12, ProductID: 7, Filename: "side.png", Position: 1},
		},
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "employee_products.tmpl", map[string]any{
		"CurrentUser":          &employee,
		"Products":             []models.Product{product},
		"EditProduct":          &product,
		"CSRFField":            template.HTML("csrf"),
		"ImageMaxMB":           5,
		"ImageLimit":           8,
		"SelectedAvailability": "all",
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
		`action="/employee/products"`,
		`name="q"`, `name="category_id"`, `name="availability"`,
		`href="/products/7"`, `href="/employee/products?edit=7#edit-product"`,
		`action="/employee/products/7/deactivate"`,
		`id="new-product"`, `value="129.99"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("employee image management does not contain %q", want)
		}
	}
}

func TestAdminCategoriesRendersTableNewAndEditControls(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	category := models.Category{Model: gorm.Model{ID: 7}, Name: "Electronics", Description: "Devices and accessories"}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "admin_categories.tmpl", map[string]any{
		"Categories": []models.Category{category}, "ViewCategory": &category, "EditCategory": &category, "DeleteCategory": &category,
		"CSRFField": template.HTML(`<input type="hidden" name="csrf">`),
	}); err != nil {
		t.Fatalf("execute admin categories template: %v", err)
	}
	body := output.String()
	for _, want := range []string{
		`id="new-category"`, `>New Category<`, `>Add category<`, `<table class="table`,
		`href="/admin/categories?view=7#view-category"`, `id="view-category"`, `>Category details<`,
		`href="/admin/categories?edit=7#edit-category"`, `id="edit-category"`,
		`action="/admin/categories/7"`, `value="Electronics"`, `>Save changes<`,
		`href="/admin/categories?delete=7#delete-category"`, `id="delete-category"`,
		`action="/admin/categories/7/delete"`, `>Confirm delete<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin categories page does not contain %q", want)
		}
	}
}

func TestEmployeeProductsTemplateRendersInactiveProductActivation(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	employee := models.User{Model: gorm.Model{ID: 2}, Name: "Employee User", Role: models.RoleEmployee}
	product := models.Product{Model: gorm.Model{ID: 8}, Name: "Inactive Product", PriceCents: 4530, Active: false}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "employee_products.tmpl", map[string]any{
		"CurrentUser": &employee, "Products": []models.Product{product}, "EditProduct": &product,
		"CSRFField": template.HTML("csrf"), "ImageMaxMB": 5, "ImageLimit": 8, "SelectedAvailability": "all",
	}); err != nil {
		t.Fatalf("execute employee products template: %v", err)
	}
	body := output.String()
	for _, want := range []string{`<h3 class="h5 mb-1">Availability</h3>`, `action="/employee/products/8/activate"`, `>Activate product</button>`, `>Activate</button>`} {
		if !strings.Contains(body, want) {
			t.Errorf("inactive product controls do not contain %q", want)
		}
	}
}

func TestDashboardLowStockAndPendingCardsAreAccessibleLinks(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     []string
	}{
		{
			name: "employee", template: "employee_dashboard.tmpl",
			data: map[string]any{"PendingOrders": int64(12), "LowStockProducts": int64(8)},
			want: []string{`class="card stat-card stat-card-link h-100" href="/employee/orders?status=pending"`, `aria-label="View 12 pending orders"`, `href="/employee/products?stock_status=low"`, `aria-label="View 8 low-stock products"`},
		},
		{
			name: "admin", template: "admin_dashboard.tmpl",
			data: map[string]any{"PendingOrders": int64(12), "LowStock": int64(8), "RevenueCents": int64(0)},
			want: []string{`class="card stat-card stat-card-link h-100" href="/admin/orders?status=pending"`, `aria-label="View 12 pending orders"`, `href="/employee/products?stock_status=low"`, `aria-label="View 8 low-stock products"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := templates.ExecuteTemplate(&output, tt.template, tt.data); err != nil {
				t.Fatalf("execute dashboard: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("dashboard does not contain %q", want)
				}
			}
		})
	}
}

func TestManagementTemplatesShowRemovableFiltersAndEmptyStates(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     []string
	}{
		{
			name: "low stock products", template: "employee_products.tmpl",
			data: map[string]any{"Products": []models.Product{}, "LowStockFilter": true, "SelectedStockStatus": "low", "SelectedAvailability": "all", "DashboardURL": "/employee/dashboard", "ImageMaxMB": 5, "ImageLimit": 8},
			want: []string{"Low Stock Products", `name="stock_status"`, `value="low" selected`, `aria-label="Remove low-stock filter"`, `href="/employee/dashboard"`, "No low-stock products."},
		},
		{
			name: "employee pending orders", template: "employee_orders.tmpl",
			data: map[string]any{"Orders": []models.Order{}, "PendingFilter": true, "SelectedStatus": "pending", "SelectedSort": "id_desc", "Statuses": []models.OrderStatus{models.OrderStatusPending}, "DashboardURL": "/employee/dashboard"},
			want: []string{"Pending Orders", `value="pending" selected`, `aria-label="Remove pending filter"`, `href="/employee/dashboard"`, "No pending orders."},
		},
		{
			name: "admin pending orders", template: "admin_orders.tmpl",
			data: map[string]any{"Orders": []models.Order{}, "PendingFilter": true, "SelectedStatus": "pending", "SelectedSort": "id_desc", "Statuses": []models.OrderStatus{models.OrderStatusPending}, "DashboardURL": "/admin/dashboard"},
			want: []string{"Pending Orders", `value="pending" selected`, `aria-label="Remove pending filter"`, `href="/admin/dashboard"`, "No pending orders."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := templates.ExecuteTemplate(&output, tt.template, tt.data); err != nil {
				t.Fatalf("execute filtered list: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("filtered list does not contain %q", want)
				}
			}
		})
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
			want:      []string{`value="preparing"`, `value="cancelled"`},
			doNotWant: []string{`value="shipped"`, `value="completed"`},
		},
		{
			name:      "ready for shipping",
			status:    models.OrderStatusReadyForShipping,
			want:      []string{"Hand over to shipping", `action="/employee/orders/7/shipment"`},
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
				"CSRFField":    template.HTML(`<input type="hidden" name="csrf">`),
				"Orders":       []models.Order{{Model: gorm.Model{ID: 7}, Status: tt.status}},
				"SelectedSort": "id_desc",
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

func TestEmployeeOrdersRendersFiltersAndSorting(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	user := models.User{Name: "Ada Lovelace", Email: "ada@example.com"}
	var output bytes.Buffer
	data := map[string]any{
		"Orders":         []models.Order{{Model: gorm.Model{ID: 11}, User: user, Status: models.OrderStatusProcessing, TotalCents: 2599}},
		"Statuses":       []models.OrderStatus{models.OrderStatusPending, models.OrderStatusProcessing},
		"UserSearch":     "ada@example.com",
		"SelectedStatus": "processing",
		"SelectedSort":   "total_desc",
		"CSRFField":      template.HTML(`<input type="hidden" name="csrf">`),
	}
	if err := templates.ExecuteTemplate(&output, "employee_orders.tmpl", data); err != nil {
		t.Fatalf("execute employee orders template: %v", err)
	}
	body := output.String()
	for _, want := range []string{
		`action="/employee/orders"`, `name="user" value="ada@example.com"`, `name="status"`, `name="sort"`,
		`value="processing" selected`, `value="total_desc" selected`, `href="/employee/orders">Reset`,
		`Ada Lovelace`, `25,99 €`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("employee orders page does not contain %q", want)
		}
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
		"Orders":          []models.Order{{Model: gorm.Model{ID: 11}, User: user, Status: models.OrderStatusProcessing}},
		"Statuses":        []models.OrderStatus{models.OrderStatusPending, models.OrderStatusProcessing},
		"UserSearch":      "ada@example.com",
		"SelectedStatus":  "processing",
		"SelectedSort":    "total_desc",
		"Page":            2,
		"TotalOrders":     int64(45),
		"TotalPages":      3,
		"PageNumbers":     []int{1, 2, 3},
		"PaginationQuery": "sort=total_desc&status=processing&user=ada%40example.com",
	}
	if err := templates.ExecuteTemplate(&output, "admin_orders.tmpl", data); err != nil {
		t.Fatalf("execute admin orders template: %v", err)
	}
	body := output.String()
	for _, want := range []string{
		`action="/admin/orders"`, `name="user" value="ada@example.com"`, `name="status"`, `name="sort"`,
		`value="processing" selected`, `value="total_desc" selected`, `href="/admin/orders">Reset`,
		`Ada Lovelace`, `ada@example.com`,
		`aria-label="Orders pagination"`, `page=1`, `page=2`, `page=3`, `aria-current="page"`,
		`Page 2 of 3`, `20 orders per page`, `1 of 45 orders`,
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

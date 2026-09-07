package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

func TestListOrdersPaginatesSortedResultsTwentyAtATime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock := newMockHandlerDatabase(t)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `orders` JOIN users ON users.id = orders.user_id.*").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(45))
	mock.ExpectQuery("SELECT .* FROM `orders` JOIN users ON users.id = orders.user_id.*ORDER BY orders.total_cents DESC, orders.id DESC LIMIT \\? OFFSET \\?").
		WithArgs(20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	handler := &AdminHandler{database: database}
	router := gin.New()
	router.SetHTMLTemplate(template.Must(template.New("admin_orders.tmpl").Parse(`{{define "admin_orders.tmpl"}}{{.Page}}|{{.TotalOrders}}|{{.TotalPages}}|{{.PaginationQuery}}{{end}}`)))
	router.GET("/admin/orders", handler.ListOrders)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/orders?sort=total_desc&page=2", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != "2|45|3|sort=total_desc&amp;status=&amp;user=" {
		t.Fatalf("unexpected pagination data %q", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestPaginationWindowUsesAtMostFivePages(t *testing.T) {
	tests := []struct {
		current int
		total   int
		want    string
	}{
		{current: 1, total: 3, want: "[1 2 3]"},
		{current: 1, total: 12, want: "[1 2 3 4 5]"},
		{current: 6, total: 12, want: "[4 5 6 7 8]"},
		{current: 12, total: 12, want: "[8 9 10 11 12]"},
	}
	for _, test := range tests {
		if got := fmt.Sprint(paginationWindow(test.current, test.total)); got != test.want {
			t.Errorf("paginationWindow(%d, %d) = %s, want %s", test.current, test.total, got, test.want)
		}
	}
}

func TestUpdateCategorySavesChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock := newMockHandlerDatabase(t)
	mock.ExpectExec("UPDATE `categories` SET .*`description`=\\?.*`name`=\\?.* WHERE id = \\?.*`deleted_at` IS NULL").
		WillReturnResult(sqlmock.NewResult(0, 1))

	handler := &AdminHandler{database: database}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/admin/categories/7", strings.NewReader("name=Updated+Category&description=Updated+description"))
	context.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	context.Params = gin.Params{{Key: "id", Value: "7"}}

	handler.UpdateCategory(context)
	context.Writer.WriteHeaderNow()

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", recorder.Code, http.StatusSeeOther, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "/admin/categories?status=updated" {
		t.Fatalf("got redirect %q", location)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDeleteUserSoftDeletesAnotherAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock := newMockHandlerDatabase(t)
	mock.ExpectQuery("SELECT \\* FROM `users`.*WHERE `users`.`id` = \\?.*LIMIT \\?").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "role", "deleted_at"}).
			AddRow(7, "Employee User", "employee@example.com", models.RoleEmployee, nil))
	mock.ExpectExec("UPDATE `users` SET `deleted_at`=\\? WHERE `users`.`id` = \\? AND `users`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	handler := &AdminHandler{database: database}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/admin/users/7/delete", nil)
	context.Params = gin.Params{{Key: "id", Value: "7"}}
	context.Set(middleware.CurrentUserKey, &models.User{Model: gorm.Model{ID: 1}, Role: models.RoleAdmin})

	handler.DeleteUser(context)
	context.Writer.WriteHeaderNow()

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusSeeOther)
	}
	if location := recorder.Header().Get("Location"); location != "/admin/users?status=deleted" {
		t.Fatalf("got redirect %q", location)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

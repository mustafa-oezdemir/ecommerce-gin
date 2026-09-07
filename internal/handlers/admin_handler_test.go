package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

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

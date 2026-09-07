package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestActivateProductMakesInactiveProductAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDatabase, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("create GORM database: %v", err)
	}
	mock.ExpectExec("UPDATE `products` SET .*`active`=\\?.* WHERE id = \\?.*`deleted_at` IS NULL").WillReturnResult(sqlmock.NewResult(0, 1))

	handler := &EmployeeHandler{database: database}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/employee/products/8/activate", nil)
	context.Params = gin.Params{{Key: "id", Value: "8"}}
	handler.ActivateProduct(context)
	context.Writer.WriteHeaderNow()

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusSeeOther)
	}
	if location := recorder.Header().Get("Location"); location != "/employee/products?edit=8#edit-product" {
		t.Fatalf("got redirect %q", location)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

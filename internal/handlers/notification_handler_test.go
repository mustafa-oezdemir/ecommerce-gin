package handlers

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNotificationMarkReadEnforcesOwnership(t *testing.T) {
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDatabase, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	other := models.User{Model: gorm.Model{ID: 9}, Email: "other@example.com", Role: models.RoleCustomer}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notifications` SET `is_read`=?,`read_at`=?,`updated_at`=? WHERE (id = ? AND user_id = ?) AND `notifications`.`deleted_at` IS NULL")).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), uint(17), other.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CurrentUserKey, &other); c.Next() })
	router.POST("/account/notifications/:id/read", NewNotificationHandler(database).MarkRead)
	request := httptest.NewRequest(http.MethodPost, "/account/notifications/"+strconv.FormatUint(17, 10)+"/read", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another user's notification, got %d", response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

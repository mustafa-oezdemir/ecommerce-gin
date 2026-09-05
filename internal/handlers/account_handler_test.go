package handlers

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
	"github.com/pquerna/otp/totp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestEmailChangeFailureReturnsActionableMessages(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantReason string
		wantText   string
	}{
		{"credentials", services.ErrInvalidCredentials, http.StatusBadRequest, "invalid_credentials", "The current password is incorrect."},
		{"input", services.ErrSecurityInput, http.StatusBadRequest, "invalid_input", "Enter a valid email address that is different from your current address."},
		{"unavailable", services.ErrEmailUnavailable, http.StatusConflict, "email_unavailable", "That email address cannot be used."},
		{"cooldown", services.ErrSecurityCooldown, http.StatusTooManyRequests, "cooldown", "Please wait before requesting another verification code."},
		{"internal", errors.New("smtp unavailable"), http.StatusServiceUnavailable, "internal_error", "The verification email could not be sent. Please try again later."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, message, reason := emailChangeFailure(test.err)
			if status != test.wantStatus || reason != test.wantReason || message != test.wantText {
				t.Fatalf("got (%d, %q, %q), want (%d, %q, %q)", status, message, reason, test.wantStatus, test.wantText, test.wantReason)
			}
		})
	}
}

func TestTwoFactorPagesRenderForAuthenticatedUserWithoutCachingSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := &gorm.DB{}
	security := services.NewAccountSecurityService(database, bytes.Repeat([]byte{0x47}, 32), nil)
	handler := NewAccountHandler(database, security, testProfileImageStore(t))
	templates, err := web.ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	user := &models.User{TwoFactorEnabled: true}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "PehliOne", AccountName: "user@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.SetHTMLTemplate(templates)
	router.GET("/account/two-factor", func(c *gin.Context) {
		c.Set(middleware.CurrentUserKey, user)
		handler.ShowTwoFactor(c)
	})
	router.GET("/account/two-factor/setup", func(c *gin.Context) {
		c.Set(middleware.CurrentUserKey, user)
		handler.renderTwoFactorSetup(c, user, &services.TwoFactorSetup{Secret: key.Secret(), URI: key.URL()}, nil)
	})

	management := httptest.NewRecorder()
	router.ServeHTTP(management, httptest.NewRequest(http.MethodGet, "/account/two-factor", nil))
	if management.Code != http.StatusOK || !strings.Contains(management.Body.String(), "Status: Enabled") {
		t.Fatalf("two-factor management response = %d: %s", management.Code, management.Body.String())
	}
	if management.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("management cache policy = %q", management.Header().Get("Cache-Control"))
	}

	setup := httptest.NewRecorder()
	router.ServeHTTP(setup, httptest.NewRequest(http.MethodGet, "/account/two-factor/setup", nil))
	if setup.Code != http.StatusOK || !strings.Contains(setup.Body.String(), "data:image/png;base64,") || !strings.Contains(setup.Body.String(), key.Secret()) {
		t.Fatalf("two-factor setup response did not contain in-memory QR and setup key: %d", setup.Code)
	}
	if setup.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("setup cache policy = %q", setup.Header().Get("Cache-Control"))
	}
}

func TestAccountShowRendersOnlyAuthenticatedUsersDatabaseValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := &gorm.DB{}
	security := services.NewAccountSecurityService(database, bytes.Repeat([]byte{0x44}, 32), nil)
	handler := NewAccountHandler(database, security, testProfileImageStore(t))
	templates, err := web.ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	user := &models.User{FirstName: "Mustafa", LastName: "Özdemir", Email: "mustafa@example.com"}
	router := gin.New()
	router.SetHTMLTemplate(templates)
	router.GET("/account", func(c *gin.Context) {
		c.Set(middleware.CurrentUserKey, user)
		handler.Show(c)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/account", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("account response = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`name="first_name" value="Mustafa"`, `name="last_name" value="Özdemir"`, `value="mustafa@example.com" readonly`} {
		if !strings.Contains(body, want) {
			t.Errorf("account response does not contain %q", want)
		}
	}
	if strings.Contains(body, "another@example.com") {
		t.Fatal("account response contains another user's data")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("account cache policy = %q", response.Header().Get("Cache-Control"))
	}
}

func TestProfileImageUploadUsesAuthenticatedUserAndRemovesOldImage(t *testing.T) {
	database, mock := newMockHandlerDatabase(t)
	directory := t.TempDir()
	store := newHandlerImageStore(t, directory, 5<<20)
	oldFilename, err := store.Save(t.Context(), "old.png", bytes.NewReader(smallPNG(t)), int64(len(smallPNG(t))))
	if err != nil {
		t.Fatalf("store old profile image: %v", err)
	}
	handler := NewAccountHandler(database, services.NewAccountSecurityService(database, bytes.Repeat([]byte{0x50}, 32), nil), store)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users` WHERE `users`.`id` = \\?").WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_image_filename"}).AddRow(7, oldFilename))
	mock.ExpectExec("UPDATE `users` SET `profile_image_filename`=\\?,`updated_at`=\\? WHERE `users`.`deleted_at` IS NULL AND `id` = \\?").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	context, response := profileImageRequest(t, "../../someone-else.png", smallPNG(t), &models.User{Model: gorm.Model{ID: 7}})
	handler.UpdateProfileImage(context)
	if context.Writer.Status() != http.StatusSeeOther || response.Header().Get("Location") != "/account?status=profile-image-updated" {
		t.Fatalf("upload response = %d location %q: %s", context.Writer.Status(), response.Header().Get("Location"), response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(directory, oldFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old image was not removed: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || !store.ValidFilename(entries[0].Name()) {
		t.Fatalf("expected one safely named new image, entries=%v error=%v", entries, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileImageDeleteOnlyUsesAuthenticatedUser(t *testing.T) {
	database, mock := newMockHandlerDatabase(t)
	directory := t.TempDir()
	store := newHandlerImageStore(t, directory, 5<<20)
	data := smallPNG(t)
	ownFilename, _ := store.Save(t.Context(), "own.png", bytes.NewReader(data), int64(len(data)))
	otherFilename, _ := store.Save(t.Context(), "other.png", bytes.NewReader(data), int64(len(data)))
	handler := NewAccountHandler(database, services.NewAccountSecurityService(database, bytes.Repeat([]byte{0x51}, 32), nil), store)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users` WHERE `users`.`id` = \\?").WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_image_filename"}).AddRow(7, ownFilename))
	mock.ExpectExec("UPDATE `users` SET `profile_image_filename`=\\?,`updated_at`=\\? WHERE `users`.`deleted_at` IS NULL AND `id` = \\?").
		WithArgs("", sqlmock.AnyArg(), uint(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/account/profile/image/delete?user_id=99", nil)
	context.Set(middleware.CurrentUserKey, &models.User{Model: gorm.Model{ID: 7}})
	handler.DeleteProfileImage(context)
	if context.Writer.Status() != http.StatusSeeOther {
		t.Fatalf("delete response = %d: %s", context.Writer.Status(), response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(directory, ownFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("own image was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, otherFilename)); err != nil {
		t.Fatalf("another user's image was affected: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileImageUploadRejectsInvalidAndOversizedFiles(t *testing.T) {
	database := &gorm.DB{}
	store := newHandlerImageStore(t, t.TempDir(), 1024)
	handler := NewAccountHandler(database, services.NewAccountSecurityService(database, bytes.Repeat([]byte{0x52}, 32), nil), store)
	tests := []struct {
		name       string
		filename   string
		data       []byte
		wantStatus int
	}{
		{name: "non image", filename: "payload.png", data: []byte("not an image"), wantStatus: http.StatusBadRequest},
		{name: "fake extension", filename: "payload.jpg", data: smallPNG(t), wantStatus: http.StatusBadRequest},
		{name: "oversized", filename: "large.png", data: make([]byte, 1025), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, response := profileImageRequest(t, test.filename, test.data, &models.User{Model: gorm.Model{ID: 7}})
			handler.UpdateProfileImage(context)
			if response.Code != test.wantStatus {
				t.Fatalf("response = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func profileImageRequest(t *testing.T, filename string, data []byte, user *models.User) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("profile_image", filename)
	if err != nil {
		t.Fatalf("create profile image part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write profile image: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart form: %v", err)
	}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/account/profile/image?user_id=99", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	context.Set(middleware.CurrentUserKey, user)
	return context, response
}

func newHandlerImageStore(t *testing.T, directory string, maxBytes int64) *uploads.ImageStore {
	t.Helper()
	store, err := uploads.NewImageStore(uploads.ImageConfig{Directory: directory, MaxBytes: maxBytes, MaxWidth: 4096, MaxHeight: 4096, MaxPixels: 16_000_000, Scanner: accountCleanScanner{}})
	if err != nil {
		t.Fatalf("create handler image store: %v", err)
	}
	return store
}

func newMockHandlerDatabase(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDatabase, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("create GORM database: %v", err)
	}
	return database, mock
}

type accountCleanScanner struct{}

func (accountCleanScanner) Scan(context.Context, []byte) error { return nil }

func testProfileImageStore(t *testing.T) *uploads.ImageStore {
	t.Helper()
	store, err := uploads.NewImageStore(uploads.ImageConfig{Directory: t.TempDir(), MaxBytes: 5 << 20, MaxWidth: 4096, MaxHeight: 4096, MaxPixels: 16_000_000, Scanner: accountCleanScanner{}})
	if err != nil {
		t.Fatalf("create profile image store: %v", err)
	}
	return store
}

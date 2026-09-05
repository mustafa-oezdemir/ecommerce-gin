package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
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
	handler := NewAccountHandler(database, security)
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
	handler := NewAccountHandler(database, security)
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

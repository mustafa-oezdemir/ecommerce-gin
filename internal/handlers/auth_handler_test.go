package handlers

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/web"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLoginCreatesNormalSessionWhenTwoFactorIsDisabled(t *testing.T) {
	testLoginSessionFlow(t, false, "/", true)
}

func TestLoginRequiresChallengeBeforeSessionWhenTwoFactorIsEnabled(t *testing.T) {
	testLoginSessionFlow(t, true, "/auth/two-factor-challenge", false)
}

func TestTwoFactorChallengeRejectsWrongCodeAndAcceptsCurrentCode(t *testing.T) {
	for _, test := range []struct {
		name          string
		code          func(string) string
		wantStatus    int
		wantLocation  string
		authenticated bool
	}{
		{name: "wrong", code: func(string) string { return "000000" }, wantStatus: http.StatusUnauthorized},
		{name: "correct", code: func(secret string) string { code, _ := totp.GenerateCode(secret, time.Now().UTC()); return code }, wantStatus: http.StatusFound, wantLocation: "/", authenticated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			sqlDatabase, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDatabase.Close()
			database, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDatabase, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.New(nilLogWriter{}, logger.Config{LogLevel: logger.Silent})})
			if err != nil {
				t.Fatal(err)
			}
			passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
			if err != nil {
				t.Fatal(err)
			}
			securityKey := bytes.Repeat([]byte{0x46}, 32)
			const secret = "JBSWY3DPEHPK3PXP"
			encryptedSecret := encryptTestSecret(t, securityKey, []byte(secret))
			mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs("user@example.com", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password", "role", "security_version", "two_factor_enabled"}).AddRow(21, "user@example.com", string(passwordHash), "customer", 4, true))
			mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs(uint(21), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "email", "role", "security_version", "two_factor_enabled", "two_factor_secret"}).AddRow(21, "user@example.com", "customer", 4, true, encryptedSecret))

			security := services.NewAccountSecurityService(database, securityKey, nil)
			handler := NewAuthHandler(database, security)
			store := cookie.NewStore([]byte("a-session-secret-that-is-long-enough"))
			router := gin.New()
			router.Use(sessions.Sessions("test_session", store))
			templates, err := web.ParseTemplates()
			if err != nil {
				t.Fatal(err)
			}
			router.SetHTMLTemplate(templates)
			router.POST("/login", handler.Login)
			router.POST("/auth/two-factor-challenge", handler.VerifyTwoFactorChallenge)
			router.GET("/session-state", func(c *gin.Context) {
				session := sessions.Default(c)
				c.JSON(http.StatusOK, gin.H{"authenticated": session.Get(middleware.SessionUserIDKey) != nil})
			})

			loginForm := url.Values{"email": {"user@example.com"}, "password": {"correct-password"}}
			loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(loginForm.Encode()))
			loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			loginResponse := httptest.NewRecorder()
			router.ServeHTTP(loginResponse, loginRequest)

			challengeForm := url.Values{"method": {"totp"}, "code": {test.code(secret)}}
			challengeRequest := httptest.NewRequest(http.MethodPost, "/auth/two-factor-challenge", strings.NewReader(challengeForm.Encode()))
			challengeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for _, cookie := range loginResponse.Result().Cookies() {
				challengeRequest.AddCookie(cookie)
			}
			challengeResponse := httptest.NewRecorder()
			router.ServeHTTP(challengeResponse, challengeRequest)
			if challengeResponse.Code != test.wantStatus || challengeResponse.Header().Get("Location") != test.wantLocation {
				t.Fatalf("challenge = %d location %q", challengeResponse.Code, challengeResponse.Header().Get("Location"))
			}
			if !test.authenticated && !strings.Contains(challengeResponse.Body.String(), "Invalid or expired authentication code") {
				t.Fatal("invalid challenge did not show a safe error")
			}
			if test.authenticated {
				stateRequest := httptest.NewRequest(http.MethodGet, "/session-state", nil)
				for _, cookie := range challengeResponse.Result().Cookies() {
					stateRequest.AddCookie(cookie)
				}
				stateResponse := httptest.NewRecorder()
				router.ServeHTTP(stateResponse, stateRequest)
				if stateResponse.Body.String() != `{"authenticated":true}` {
					t.Fatalf("post-challenge state = %s", stateResponse.Body.String())
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func testLoginSessionFlow(t *testing.T, twoFactorEnabled bool, expectedLocation string, expectAuthenticated bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDatabase, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.New(nilLogWriter{}, logger.Config{LogLevel: logger.Silent})})
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs("user@example.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password", "role", "security_version", "two_factor_enabled"}).AddRow(21, "user@example.com", string(passwordHash), "customer", 4, twoFactorEnabled))

	security := services.NewAccountSecurityService(database, bytes.Repeat([]byte{0x45}, 32), nil)
	handler := NewAuthHandler(database, security)
	store := cookie.NewStore([]byte("a-session-secret-that-is-long-enough"))
	router := gin.New()
	router.Use(sessions.Sessions("test_session", store))
	router.POST("/login", handler.Login)
	router.GET("/session-state", func(c *gin.Context) {
		session := sessions.Default(c)
		c.JSON(http.StatusOK, gin.H{
			"authenticated": session.Get(middleware.SessionUserIDKey) != nil,
			"challenge":     session.Get(sessionTwoFactorUserID) != nil,
		})
	})

	form := url.Values{"email": {"user@example.com"}, "password": {"correct-password"}}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != expectedLocation {
		t.Fatalf("login = %d location %q, want %q", response.Code, response.Header().Get("Location"), expectedLocation)
	}

	stateRequest := httptest.NewRequest(http.MethodGet, "/session-state", nil)
	for _, cookie := range response.Result().Cookies() {
		stateRequest.AddCookie(cookie)
	}
	stateResponse := httptest.NewRecorder()
	router.ServeHTTP(stateResponse, stateRequest)
	state := stateResponse.Body.String()
	if expectAuthenticated && state != `{"authenticated":true,"challenge":false}` {
		t.Fatalf("normal login session state = %s", state)
	}
	if !expectAuthenticated && state != `{"authenticated":false,"challenge":true}` {
		t.Fatalf("two-factor login session state = %s", state)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type nilLogWriter struct{}

func (nilLogWriter) Printf(string, ...any) {}

var _ logger.Writer = nilLogWriter{}

func encryptTestSecret(t *testing.T, key, plain []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	return gcm.Seal(nonce, nonce, plain, nil)
}

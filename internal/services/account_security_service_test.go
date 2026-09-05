package services

import (
	"bytes"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAccountSecurityEncryptionRoundTrip(t *testing.T) {
	service := NewAccountSecurityService(&gorm.DB{}, bytes.Repeat([]byte{0x42}, 32), nil)
	plain := []byte("JBSWY3DPEHPK3PXP")
	ciphertext, err := service.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(ciphertext, plain) {
		t.Fatal("ciphertext contains the TOTP secret")
	}
	decoded, err := service.decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatalf("round trip mismatch: %q", decoded)
	}
}

func TestUpdateProfileUsesAuthenticatedUserID(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `first_name`=?,`last_name`=?,`name`=?,`updated_at`=? WHERE id = ? AND `users`.`deleted_at` IS NULL")).
		WithArgs("Mustafa", "Özdemir", "Mustafa Özdemir", sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `security_version` FROM `users`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"security_version"}).AddRow(uint64(3)))

	version, err := service.UpdateProfile(t.Context(), 7, "  Mustafa ", " Özdemir ")
	if err != nil || version != 3 {
		t.Fatalf("update profile = version %d, error %v", version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	stored, err := bcrypt.GenerateFromPassword([]byte("correct-current-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "password", "security_version"}).AddRow(9, string(stored), 1))

	if _, err := service.ChangePassword(t.Context(), 9, "wrong-password", "new-password-123", "new-password-123"); err != ErrInvalidCredentials {
		t.Fatalf("wrong current password error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestChangePasswordStoresBcryptHashNotPlaintext(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	stored, err := bcrypt.GenerateFromPassword([]byte("correct-current-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "password", "security_version"}).AddRow(9, string(stored), 1))
	mock.ExpectExec("UPDATE `users` SET `password`=\\?,`security_version`=security_version \\+ 1,`updated_at`=\\?").
		WithArgs(bcryptHashArgument{password: "new-password-123"}, sqlmock.AnyArg(), uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `security_version` FROM `users`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"security_version"}).AddRow(uint64(2)))

	version, err := service.ChangePassword(t.Context(), 9, "correct-current-password", "new-password-123", "new-password-123")
	if err != nil || version != 2 {
		t.Fatalf("change password = version %d, error %v", version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPValidationAcceptsCurrentCodeAndRejectsInvalidCode(t *testing.T) {
	service := NewAccountSecurityService(&gorm.DB{}, bytes.Repeat([]byte{0x61}, 32), nil)
	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	const secret = "JBSWY3DPEHPK3PXP"
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatalf("encrypt TOTP secret: %v", err)
	}
	code, err := totp.GenerateCode(secret, now)
	if err != nil {
		t.Fatalf("generate TOTP code: %v", err)
	}
	if !service.validateTOTP(encrypted, code) {
		t.Fatal("current TOTP code was rejected")
	}
	if service.validateTOTP(encrypted, "000000") {
		t.Fatal("invalid TOTP code was accepted")
	}
}

func TestBeginTwoFactorGeneratesAndEncryptsSecret(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	var storedSecret []byte
	mock.ExpectExec("UPDATE `users` SET `two_factor_confirmed_at`=\\?,`two_factor_enabled`=\\?,`two_factor_secret`=\\?,`updated_at`=\\?").
		WithArgs(nil, false, encryptedBytesArgument{captured: &storedSecret}, sqlmock.AnyArg(), uint(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	setup, err := service.BeginTwoFactor(t.Context(), models.User{Model: gorm.Model{ID: 4}, Email: "user@example.com"})
	if err != nil {
		t.Fatalf("begin two-factor: %v", err)
	}
	if setup.Secret == "" || !strings.HasPrefix(setup.URI, "otpauth://totp/") {
		t.Fatalf("invalid setup: %#v", setup)
	}
	plain, err := service.decrypt(storedSecret)
	if err != nil || string(plain) != setup.Secret {
		t.Fatalf("stored secret did not decrypt to setup secret: %q, %v", plain, err)
	}
	if bytes.Contains(storedSecret, []byte(setup.Secret)) {
		t.Fatal("stored TOTP value contains plaintext secret")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmTwoFactorEnablesAccountWithValidCode(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	now := time.Date(2026, time.September, 5, 13, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	const secret = "JBSWY3DPEHPK3PXP"
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs(uint(12), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "two_factor_enabled", "two_factor_secret", "security_version"}).AddRow(12, false, encrypted, 1))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `recovery_codes` WHERE user_id = \\?").WithArgs(uint(12)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `recovery_codes`").WillReturnResult(sqlmock.NewResult(1, recoveryCodeCount))
	mock.ExpectExec("UPDATE `users` SET `security_version`=security_version \\+ 1,.*`two_factor_enabled`=\\?").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT `security_version` FROM `users`").WithArgs(uint(12), 1).
		WillReturnRows(sqlmock.NewRows([]string{"security_version"}).AddRow(uint64(2)))

	codes, version, err := service.ConfirmTwoFactor(t.Context(), 12, code)
	if err != nil || version != 2 || len(codes) != recoveryCodeCount {
		t.Fatalf("confirm two-factor = %d codes, version %d, error %v", len(codes), version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmTwoFactorRejectsInvalidCode(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	encrypted, err := service.encrypt([]byte("JBSWY3DPEHPK3PXP"))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs(uint(12), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "two_factor_enabled", "two_factor_secret"}).AddRow(12, false, encrypted))
	if _, _, err := service.ConfirmTwoFactor(t.Context(), 12, "not-a-valid-code"); err != ErrInvalidSecurityCode {
		t.Fatalf("invalid TOTP error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDisableTwoFactorClearsSecretAndRecoveryCodes(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	now := time.Date(2026, time.September, 5, 14, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	const secret = "JBSWY3DPEHPK3PXP"
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("current-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs(uint(15), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "password", "two_factor_enabled", "two_factor_secret", "security_version"}).AddRow(15, string(passwordHash), true, encrypted, 1))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `recovery_codes` WHERE user_id = \\?").WithArgs(uint(15)).WillReturnResult(sqlmock.NewResult(0, recoveryCodeCount))
	mock.ExpectExec("UPDATE `users` SET `security_version`=security_version \\+ 1,.*`two_factor_enabled`=\\?.*`two_factor_secret`=\\?").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT `security_version` FROM `users`").WithArgs(uint(15), 1).
		WillReturnRows(sqlmock.NewRows([]string{"security_version"}).AddRow(uint64(2)))

	version, err := service.DisableTwoFactor(t.Context(), 15, "current-password", code)
	if err != nil || version != 2 {
		t.Fatalf("disable two-factor = version %d, error %v", version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDisableTwoFactorRejectsWrongPassword(t *testing.T) {
	service, mock := newMockAccountSecurityService(t)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("current-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM `users`").WithArgs(uint(15), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "password", "two_factor_enabled", "two_factor_secret"}).AddRow(15, string(passwordHash), true, []byte("encrypted")))
	if _, err := service.DisableTwoFactor(t.Context(), 15, "wrong-password", "123456"); err != ErrInvalidCredentials {
		t.Fatalf("wrong disable password error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryCodesAreUniqueReadableAndStoredAsHashes(t *testing.T) {
	service := NewAccountSecurityService(&gorm.DB{}, bytes.Repeat([]byte{0x24}, 32), nil)
	codes, hashes, err := service.newRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	if len(codes) != recoveryCodeCount || len(hashes) != recoveryCodeCount {
		t.Fatalf("got %d codes and %d hashes", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for index, code := range codes {
		if len(code) != 14 || strings.Count(code, "-") != 2 {
			t.Fatalf("unreadable recovery code %q", code)
		}
		if seen[code] {
			t.Fatalf("duplicate recovery code %q", code)
		}
		seen[code] = true
		if len(hashes[index]) != 32 || bytes.Equal(hashes[index], []byte(normalizeCode(code))) {
			t.Fatal("recovery code was not hashed")
		}
	}
}

func TestReadableEmailCodeFormat(t *testing.T) {
	code, err := randomReadableCode()
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if len(code) != 7 || code[3] != '-' {
		t.Fatalf("unexpected code format %q", code)
	}
	for _, character := range strings.ReplaceAll(code, "-", "") {
		if strings.ContainsRune("01OIL", character) {
			t.Fatalf("ambiguous character in %q", code)
		}
	}
}

func TestEmailCodeHashUsesNormalizedReadableCode(t *testing.T) {
	service := NewAccountSecurityService(&gorm.DB{}, bytes.Repeat([]byte{0x55}, 32), nil)
	stored := service.hashCode(normalizeCode("K7M-42P"))
	entered := service.hashCode(normalizeCode("k7m 42p"))
	if !bytes.Equal(stored, entered) {
		t.Fatal("formatted security code did not normalize to the stored hash")
	}
}

func TestAccountSecurityRejectsWeakOrMismatchedPasswordBeforeDatabase(t *testing.T) {
	service := NewAccountSecurityService(&gorm.DB{}, bytes.Repeat([]byte{0x33}, 32), nil)
	if _, err := service.ChangePassword(t.Context(), 1, "old", "short", "short"); err != ErrSecurityInput {
		t.Fatalf("weak password error = %v", err)
	}
	if _, err := service.ChangePassword(t.Context(), 1, "old", "long-enough-password", "different-password"); err != ErrSecurityInput {
		t.Fatalf("confirmation error = %v", err)
	}
}

type bcryptHashArgument struct{ password string }

func (argument bcryptHashArgument) Match(value driver.Value) bool {
	hash, ok := value.(string)
	return ok && hash != argument.password && bcrypt.CompareHashAndPassword([]byte(hash), []byte(argument.password)) == nil
}

type encryptedBytesArgument struct{ captured *[]byte }

func (argument encryptedBytesArgument) Match(value driver.Value) bool {
	bytesValue, ok := value.([]byte)
	if !ok || len(bytesValue) < 32 {
		return false
	}
	*argument.captured = append((*argument.captured)[:0], bytesValue...)
	return true
}

func newMockAccountSecurityService(t *testing.T) (*AccountSecurityService, sqlmock.Sqlmock) {
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
	return NewAccountSecurityService(database, bytes.Repeat([]byte{0x73}, 32), nil), mock
}

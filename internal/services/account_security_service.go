package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	recoveryCodeCount = 8
	maxEmailAttempts  = 5
	emailCodeLifetime = 10 * time.Minute
	emailCodeCooldown = time.Minute
)

var (
	ErrSecurityInput       = errors.New("invalid security input")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidSecurityCode = errors.New("invalid or expired security code")
	ErrSecurityCooldown    = errors.New("security code was requested too recently")
	ErrEmailUnavailable    = errors.New("email address is unavailable")
	ErrTwoFactorDisabled   = errors.New("two-factor authentication is not enabled")
)

type SecurityMailer interface {
	SendSecurityCode(to, displayName, code string, expiresIn time.Duration) error
	SendPasswordChanged(user models.User)
}

type TwoFactorSetup struct {
	Secret string
	URI    string
}

type AccountSecurityService struct {
	database *gorm.DB
	key      []byte
	mailer   SecurityMailer
	now      func() time.Time
}

func NewAccountSecurityService(database *gorm.DB, key []byte, mailer SecurityMailer) *AccountSecurityService {
	if database == nil || len(key) != 32 {
		panic("services: account security database and 32-byte key are required")
	}
	return &AccountSecurityService{database: database, key: append([]byte(nil), key...), mailer: mailer, now: time.Now}
}

func (s *AccountSecurityService) UpdateProfile(ctx context.Context, userID uint, firstName, lastName string) (uint64, error) {
	firstName, lastName = strings.TrimSpace(firstName), strings.TrimSpace(lastName)
	if userID == 0 || utf8.RuneCountInString(firstName) < 1 || utf8.RuneCountInString(firstName) > 100 || utf8.RuneCountInString(lastName) < 1 || utf8.RuneCountInString(lastName) > 100 {
		return 0, ErrSecurityInput
	}
	name := strings.TrimSpace(firstName + " " + lastName)
	if err := s.database.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{"first_name": firstName, "last_name": lastName, "name": name}).Error; err != nil {
		return 0, fmt.Errorf("update profile: %w", err)
	}
	return s.securityVersion(ctx, userID)
}

func (s *AccountSecurityService) ChangePassword(ctx context.Context, userID uint, current, password, confirmation string) (uint64, error) {
	if userID == 0 || len(password) < 12 || len(password) > 72 || password != confirmation {
		return 0, ErrSecurityInput
	}
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil {
		return 0, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(current)) != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil {
		return 0, ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash password: %w", err)
	}
	if err := s.database.WithContext(ctx).Model(&user).Updates(map[string]any{"password": string(hash), "security_version": gorm.Expr("security_version + 1")}).Error; err != nil {
		return 0, fmt.Errorf("change password: %w", err)
	}
	if s.mailer != nil {
		go s.mailer.SendPasswordChanged(user)
	}
	slog.Info("account security event", "event", "password_changed", "user_id", userID)
	return s.securityVersion(ctx, userID)
}

func (s *AccountSecurityService) BeginTwoFactor(ctx context.Context, user models.User) (*TwoFactorSetup, error) {
	if user.ID == 0 || user.TwoFactorEnabled {
		return nil, ErrSecurityInput
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "PehliOne", AccountName: user.Email, Period: 30, SecretSize: 20, Secret: nil, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return nil, fmt.Errorf("generate TOTP: %w", err)
	}
	encrypted, err := s.encrypt([]byte(key.Secret()))
	if err != nil {
		return nil, err
	}
	if err := s.database.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{"two_factor_secret": encrypted, "two_factor_enabled": false, "two_factor_confirmed_at": nil}).Error; err != nil {
		return nil, fmt.Errorf("store TOTP secret: %w", err)
	}
	return &TwoFactorSetup{Secret: key.Secret(), URI: key.URL()}, nil
}

func (s *AccountSecurityService) PendingTwoFactor(ctx context.Context, userID uint) (*TwoFactorSetup, error) {
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || len(user.TwoFactorSecret) == 0 || user.TwoFactorEnabled {
		return nil, ErrSecurityInput
	}
	secret, err := s.decrypt(user.TwoFactorSecret)
	if err != nil {
		return nil, err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "PehliOne", AccountName: user.Email, Period: 30, SecretSize: 20, Secret: secret, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return nil, fmt.Errorf("restore TOTP key: %w", err)
	}
	return &TwoFactorSetup{Secret: string(secret), URI: key.URL()}, nil
}

func (s *AccountSecurityService) ConfirmTwoFactor(ctx context.Context, userID uint, code string) ([]string, uint64, error) {
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || len(user.TwoFactorSecret) == 0 || user.TwoFactorEnabled {
		return nil, 0, ErrSecurityInput
	}
	if !s.validateTOTP(user.TwoFactorSecret, code) {
		return nil, 0, ErrInvalidSecurityCode
	}
	codes, hashes, err := s.newRecoveryCodes()
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	err = s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		rows := make([]models.RecoveryCode, len(hashes))
		for i := range hashes {
			rows[i] = models.RecoveryCode{UserID: userID, CodeHash: hashes[i]}
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{"two_factor_enabled": true, "two_factor_confirmed_at": now, "security_version": gorm.Expr("security_version + 1")}).Error
	})
	if err != nil {
		return nil, 0, fmt.Errorf("enable two-factor authentication: %w", err)
	}
	slog.Info("account security event", "event", "two_factor_enabled", "user_id", userID)
	version, err := s.securityVersion(ctx, userID)
	return codes, version, err
}

func (s *AccountSecurityService) VerifySecondFactor(ctx context.Context, userID uint, code string, recovery bool) (*models.User, error) {
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || !user.TwoFactorEnabled {
		return nil, ErrTwoFactorDisabled
	}
	if recovery {
		hash := s.hashCode(normalizeCode(code))
		now := s.now()
		result := s.database.WithContext(ctx).Model(&models.RecoveryCode{}).Where("user_id = ? AND code_hash = ? AND used_at IS NULL", userID, hash).Update("used_at", now)
		if result.Error != nil || result.RowsAffected != 1 {
			return nil, ErrInvalidSecurityCode
		}
		slog.Info("account security event", "event", "recovery_code_used", "user_id", userID)
	} else if !s.validateTOTP(user.TwoFactorSecret, code) {
		return nil, ErrInvalidSecurityCode
	}
	return &user, nil
}

func (s *AccountSecurityService) RegenerateRecoveryCodes(ctx context.Context, userID uint, password string) ([]string, uint64, error) {
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || !user.TwoFactorEnabled || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return nil, 0, ErrInvalidCredentials
	}
	codes, hashes, err := s.newRecoveryCodes()
	if err != nil {
		return nil, 0, err
	}
	err = s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		rows := make([]models.RecoveryCode, len(hashes))
		for i := range hashes {
			rows[i] = models.RecoveryCode{UserID: userID, CodeHash: hashes[i]}
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", userID).Update("security_version", gorm.Expr("security_version + 1")).Error
	})
	if err != nil {
		return nil, 0, err
	}
	slog.Info("account security event", "event", "recovery_codes_regenerated", "user_id", userID)
	version, err := s.securityVersion(ctx, userID)
	return codes, version, err
}

func (s *AccountSecurityService) DisableTwoFactor(ctx context.Context, userID uint, password, code string) (uint64, error) {
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || !user.TwoFactorEnabled || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return 0, ErrInvalidCredentials
	}
	if !s.validateTOTP(user.TwoFactorSecret, code) {
		return 0, ErrInvalidSecurityCode
	}
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{"two_factor_enabled": false, "two_factor_secret": nil, "two_factor_confirmed_at": nil, "security_version": gorm.Expr("security_version + 1")}).Error
	})
	if err != nil {
		return 0, err
	}
	slog.Info("account security event", "event", "two_factor_disabled", "user_id", userID)
	return s.securityVersion(ctx, userID)
}

func (s *AccountSecurityService) RequestEmailChange(ctx context.Context, userID uint, password, pendingEmail string) error {
	pendingEmail = strings.ToLower(strings.TrimSpace(pendingEmail))
	if userID == 0 || len(pendingEmail) > 254 || !strings.Contains(pendingEmail, "@") {
		return ErrSecurityInput
	}
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	if strings.EqualFold(user.Email, pendingEmail) {
		return ErrSecurityInput
	}
	var count int64
	if err := s.database.WithContext(ctx).Model(&models.User{}).Where("LOWER(email) = ? AND id <> ?", pendingEmail, userID).Count(&count).Error; err != nil {
		return fmt.Errorf("check email availability: %w", err)
	}
	if count != 0 {
		return ErrEmailUnavailable
	}
	var previous models.EmailChangeRequest
	if err := s.database.WithContext(ctx).Where("user_id = ?", userID).First(&previous).Error; err == nil && s.now().Sub(previous.CreatedAt) < emailCodeCooldown {
		return ErrSecurityCooldown
	}
	code, err := randomReadableCode()
	if err != nil {
		return err
	}
	request := models.EmailChangeRequest{UserID: userID, PendingEmail: pendingEmail, CodeHash: s.hashCode(normalizeCode(code)), ExpiresAt: s.now().Add(emailCodeLifetime)}
	if err := s.database.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"pending_email", "code_hash", "expires_at", "attempts", "created_at", "updated_at"})}).Create(&request).Error; err != nil {
		return fmt.Errorf("store email verification: %w", err)
	}
	if s.mailer != nil {
		if err := s.mailer.SendSecurityCode(pendingEmail, user.Name, code, emailCodeLifetime); err != nil {
			_ = s.database.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.EmailChangeRequest{}).Error
			return fmt.Errorf("send email verification: %w", err)
		}
	}
	slog.Info("account security event", "event", "email_change_requested", "user_id", userID)
	return nil
}

func (s *AccountSecurityService) ConfirmEmailChange(ctx context.Context, userID uint, code string) (uint64, error) {
	var request models.EmailChangeRequest
	if err := s.database.WithContext(ctx).Where("user_id = ?", userID).First(&request).Error; err != nil {
		return 0, ErrInvalidSecurityCode
	}
	if !s.now().Before(request.ExpiresAt) || request.Attempts >= maxEmailAttempts || !hmac.Equal(request.CodeHash, s.hashCode(normalizeCode(code))) {
		_ = s.database.WithContext(ctx).Model(&request).Update("attempts", gorm.Expr("attempts + 1")).Error
		return 0, ErrInvalidSecurityCode
	}
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Where("LOWER(email) = ? AND id <> ?", request.PendingEmail, userID).Count(&count).Error; err != nil {
			return fmt.Errorf("check email availability: %w", err)
		}
		if count != 0 {
			return ErrEmailUnavailable
		}
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{"email": request.PendingEmail, "security_version": gorm.Expr("security_version + 1")}).Error; err != nil {
			return err
		}
		return tx.Delete(&request).Error
	})
	if err != nil {
		return 0, err
	}
	slog.Info("account security event", "event", "email_changed", "user_id", userID)
	return s.securityVersion(ctx, userID)
}

func (s *AccountSecurityService) DeleteAccount(ctx context.Context, userID uint, password string) error {
	var user models.User
	if err := s.database.WithContext(ctx).First(&user, userID).Error; err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	if err := s.database.WithContext(ctx).Delete(&user).Error; err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	slog.Info("account security event", "event", "account_deleted", "user_id", userID)
	return nil
}

func (s *AccountSecurityService) validateTOTP(encrypted []byte, code string) bool {
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return false
	}
	ok, err := totp.ValidateCustom(normalizeCode(code), string(secret), s.now().UTC(), totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	return err == nil && ok
}

func (s *AccountSecurityService) newRecoveryCodes() ([]string, [][]byte, error) {
	codes, hashes := make([]string, recoveryCodeCount), make([][]byte, recoveryCodeCount)
	for i := range codes {
		bytes := make([]byte, 8)
		if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		raw := strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), "=")
		codes[i] = raw[:4] + "-" + raw[4:8] + "-" + raw[8:12]
		hashes[i] = s.hashCode(normalizeCode(codes[i]))
	}
	return codes, hashes, nil
}

func (s *AccountSecurityService) hashCode(code string) []byte {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (s *AccountSecurityService) encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (s *AccountSecurityService) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted secret")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func (s *AccountSecurityService) securityVersion(ctx context.Context, userID uint) (uint64, error) {
	var user models.User
	if err := s.database.WithContext(ctx).Select("security_version").First(&user, userID).Error; err != nil {
		return 0, err
	}
	return user.SecurityVersion, nil
}

func randomReadableCode() (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	result := make([]byte, 7)
	for i := range result {
		if i == 3 {
			result[i] = '-'
			continue
		}
		value, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		result[i] = alphabet[value.Int64()]
	}
	return string(result), nil
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(value)))
}

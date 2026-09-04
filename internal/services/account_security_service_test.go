package services

import (
	"bytes"
	"strings"
	"testing"

	"gorm.io/gorm"
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

func TestAccountSecurityRejectsWeakOrMismatchedPasswordBeforeDatabase(t *testing.T) {
	service := NewAccountSecurityService(&gorm.DB{}, bytes.Repeat([]byte{0x33}, 32), nil)
	if _, err := service.ChangePassword(t.Context(), 1, "old", "short", "short"); err != ErrSecurityInput {
		t.Fatalf("weak password error = %v", err)
	}
	if _, err := service.ChangePassword(t.Context(), 1, "old", "long-enough-password", "different-password"); err != ErrSecurityInput {
		t.Fatalf("confirmation error = %v", err)
	}
}

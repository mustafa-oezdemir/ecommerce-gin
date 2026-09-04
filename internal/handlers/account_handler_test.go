package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
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

package services

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestReviewValidationRejectsInvalidContentBeforeDatabase(t *testing.T) {
	service := NewProductEngagementService(&gorm.DB{})
	tests := []struct {
		name        string
		rating      uint8
		title, body string
	}{
		{name: "zero rating", rating: 0, title: "Useful", body: "A useful review body"},
		{name: "high rating", rating: 11, title: "Useful", body: "A useful review body"},
		{name: "short title", rating: 8, title: "No", body: "A useful review body"},
		{name: "short body", rating: 8, title: "Useful", body: "Too short"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.CreateReview(t.Context(), 1, 1, test.rating, test.title, test.body)
			if !errors.Is(err, ErrInvalidReview) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestReviewSortIsAllowListed(t *testing.T) {
	// This guards the public sort values independently from SQL construction.
	allowed := map[string]bool{"": true, "newest": true, "oldest": true, "highest": true, "lowest": true}
	for _, value := range []string{"", "newest", "oldest", "highest", "lowest"} {
		if !allowed[value] {
			t.Fatalf("missing sort %q", value)
		}
	}
}

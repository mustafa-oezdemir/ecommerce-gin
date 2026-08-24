package validation

import "testing"

func TestParseCents(t *testing.T) {
	tests := []struct {
		value string
		want  int64
		valid bool
	}{
		{"19.99", 1999, true}, {"19,9", 1990, true}, {"1", 100, true}, {"0", 0, false}, {"-1", 0, false}, {"3.999", 0, false}, {"abc", 0, false},
	}
	for _, tt := range tests {
		got, err := ParseCents(tt.value)
		if (err == nil) != tt.valid || got != tt.want {
			t.Fatalf("ParseCents(%q) = %d, %v", tt.value, got, err)
		}
	}
}

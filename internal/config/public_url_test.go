package config

import "testing"

func TestValidatePublicURL(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		production bool
		wantURL    string
		wantHost   string
		wantError  bool
	}{
		{name: "production domain", raw: "http://localhost:8080/", production: true, wantURL: "http://localhost:8080", wantHost: "localhost:8080"},
		{name: "local development", raw: "http://localhost:8080", wantURL: "http://localhost:8080", wantHost: "localhost:8080"},
		{name: "production rejects plain HTTP", raw: "http://localhost:8080", production: true, wantError: true},
		{name: "rejects path", raw: "https://localhost:8080/store", production: true, wantError: true},
		{name: "rejects credentials", raw: "https://user:pass@localhost:8080", production: true, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotURL, gotHost, err := validatePublicURL("APP_URL", test.raw, test.production)
			if test.wantError {
				if err == nil {
					t.Fatal("expected validation error")
				}
				return
			}
			if err != nil || gotURL != test.wantURL || gotHost != test.wantHost {
				t.Fatalf("validatePublicURL() = %q, %q, %v", gotURL, gotHost, err)
			}
		})
	}
}

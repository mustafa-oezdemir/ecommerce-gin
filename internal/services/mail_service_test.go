package services

import "testing"

func TestOrderURLUsesCanonicalApplicationURL(t *testing.T) {
	for _, test := range []struct{ name, appURL, want string }{
		{name: "development", appURL: "http://localhost:8080", want: "http://localhost:8080/account/orders/42"},
		{name: "production target", appURL: "https://pehlione-ecommerce.com", want: "https://pehlione-ecommerce.com/account/orders/42"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &MailService{appURL: test.appURL}
			if got := service.orderURL(42); got != test.want {
				t.Fatalf("unexpected order URL: %s", got)
			}
		})
	}
}

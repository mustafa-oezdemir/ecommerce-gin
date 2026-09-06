package services

import "testing"

func TestOrderURLUsesCanonicalApplicationURL(t *testing.T) {
	service := &MailService{appURL: "https://pehlione-ecommerce.com"}
	if got := service.orderURL(42); got != "https://pehlione-ecommerce.com/account/orders/42" {
		t.Fatalf("unexpected order URL: %s", got)
	}
}

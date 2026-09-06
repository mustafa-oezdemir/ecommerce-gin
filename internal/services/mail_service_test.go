package services

import "testing"

func TestOrderURLUsesCanonicalApplicationURL(t *testing.T) {
	service := &MailService{appURL: "http://localhost:8080"}
	if got := service.orderURL(42); got != "https://localhost:8080/account/orders/42" {
		t.Fatalf("unexpected order URL: %s", got)
	}
}

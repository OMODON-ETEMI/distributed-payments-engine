package routes

import (
	"context"
	"testing"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
)

func TestPaymentRouter_RouteAndSignature(t *testing.T) {
	// Provider 1 always fails, provider 2 always succeeds
	p1 := routes.NewMockProvider("p1", 1.0)
	p2 := routes.NewMockProvider("paystack", 0.0)

	pb1 := routes.NewProviderBreaker(p1, routes.BreakerConfig{MaxRequests: 1, Interval: time.Second, Timeout: time.Second, ConsecutiveFailThreshold: 1})
	pb2 := routes.NewProviderBreaker(p2, routes.BreakerConfig{MaxRequests: 1, Interval: time.Second, Timeout: time.Second, ConsecutiveFailThreshold: 1})

	router := routes.NewPaymentRouter([]*routes.ProviderBreaker{pb1, pb2})

	req := routes.InitiateRequest{Amount: "100", Currency: "NGN", RecipientCode: "R", Reference: "ref", Reason: "test"}
	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("expected a provider to succeed, got error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if resp.Status != "pending" {
		t.Fatalf("unexpected provider response status: %s", resp.Status)
	}

	// Verify signature checks provider by name and delegates correctly
	ok := router.VerifySignature("paystack", []byte("payload"), "mock-signature")
	if !ok {
		t.Fatalf("expected signature to validate for mock-signature")
	}
	notOk := router.VerifySignature("paystack", []byte("payload"), "bad")
	if notOk {
		t.Fatalf("expected invalid signature to fail")
	}

	statuses := router.ProviderStatuses()
	if s, ok := statuses["paystack"]; !ok || s == "" {
		t.Fatalf("expected provider status for paystack to be present, got: %v", statuses)
	}
}

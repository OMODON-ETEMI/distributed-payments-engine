package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http/httptest"
	"time"

	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/google/uuid"
	"github.com/sony/gobreaker"
)

type PaymentProvider interface {
	Name() string
	InitiateTransfer(ctx context.Context, req internal.InitiateRequest) (*internal.InitiateResponse, error)
	VerifyWebhookSignature(payload []byte, signature string) bool
}

// ── Mock provider ────────────────────────────────────────────────────

type MockProvider struct {
	name        string
	failureRate float64 // 0.0 = never fails, 1.0 = always fails
	Api         *ApiConfig
}

func NewMockProvider(name string, failureRate float64) *MockProvider {
	return &MockProvider{name: name, failureRate: failureRate}
}

func (m *MockProvider) Name() string { return m.name }

func (m *MockProvider) InitiateTransfer(ctx context.Context, req internal.InitiateRequest) (*internal.InitiateResponse, error) {
	// Simulate network latency
	time.Sleep(time.Duration(rand.Intn(200)+50) * time.Millisecond)

	// Simulate provider being down
	if rand.Float64() < m.failureRate {
		return nil, fmt.Errorf("provider %s: service unavailable (simulated)", m.name)
	}

	// Return a fake provider reference — this is what gets stored
	// as external_reference on the transfer_request
	resp := &internal.InitiateResponse{
		ProviderReference: fmt.Sprintf("MOCK-%s-%s", m.name, uuid.NewString()[:8]),
		Status:            "pending",
		QueuedAt:          time.Now(),
	}

	go func() {
		m.TransferResponse(req)
	}()

	return resp, nil
}

func (m *MockProvider) VerifyWebhookSignature(payload []byte, signature string) bool {
	// Mock always accepts "mock-signature" as valid
	return signature == "mock-signature"
}

func (m *MockProvider) TransferResponse(req internal.InitiateRequest) {
	// Simulate network latency
	time.Sleep(2 * time.Second)

	webhookTransferID := fmt.Sprintf("MOCK-WEBHOOK-TRF-%s", uuid.NewString()[:8]) // Unique ID for the webhook event itself

	var webhookStatus string
	var webhookEvent string
	var failureReason string

	if rand.Float64() < m.failureRate {
		// Simulate failure
		failureType := rand.Intn(2) // 0 for failed, 1 for reversed
		if failureType == 0 {
			webhookStatus = "failed"
			webhookEvent = "transfer.failed"
			failureReason = "simulated_failure"
		} else {
			webhookStatus = "reversed"
			webhookEvent = "transfer.reversed"
			failureReason = "simulated_reversal"
		}
	} else {
		// Simulate success
		webhookStatus = "success"
		webhookEvent = "transfer.success"
	}

	transferData, err := json.Marshal(&internal.WebhookTransferData{
		ID:            webhookTransferID, // This is the webhook's own ID for the transfer event
		Reference:     req.Reference,     // This is the original transfer_request_id from our system
		Provider:      "paystack",
		Status:        webhookStatus,
		BankCode:      "044",
		FullName:      "Alexis Sanchez", // Hardcoded for mock, could be dynamic
		Amount:        req.Amount,
		Currency:      req.Currency,
		FailureReason: failureReason,
	})
	if err != nil {
		log.Printf("Error marshaling WebhookTransferData: %v", err)
		return
	}

	payload, err := json.Marshal(&internal.WebhookBody{
		Provider: "paystack",
		Event:    webhookEvent,
		Type:     "withdrawal.webhook",
		ID:       webhookTransferID, // Webhook's own ID
		Data:     transferData,
	})
	if err != nil {
		log.Printf("Error marshaling WebhookBody: %v", err)
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Internal Webhook call panicked: %v", r)
			}
		}()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/webhook/paystack", bytes.NewReader(payload))
		r.Header.Set("X-Paystack-Signature", "mock-signature")

		if m.Api == nil {
			log.Println("MockProvider: ApiConfig is nil, cannot call webhook")
			return
		}

		m.Api.HandlePaystackWebhook(w, r)
		slog.Info("Internal webhook simulation finished", "status", webhookStatus, "event", webhookEvent, "response", w.Body.String())
	}()
}

// ── Circuit breaker wrapper (from before) ───────────────────────────

type ProviderBreaker struct {
	Provider PaymentProvider
	breaker  *gobreaker.CircuitBreaker
}

func NewProviderBreaker(p PaymentProvider, cfg internal.BreakerConfig) *ProviderBreaker {
	settings := gobreaker.Settings{
		Name:        p.Name(),
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailThreshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Info("circuit breaker state changed",
				"provider", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	}
	return &ProviderBreaker{
		Provider: p,
		breaker:  gobreaker.NewCircuitBreaker(settings),
	}
}

func (pb *ProviderBreaker) InitiateTransfer(ctx context.Context, req internal.InitiateRequest) (*internal.InitiateResponse, error) {
	result, err := pb.breaker.Execute(func() (interface{}, error) {
		return pb.Provider.InitiateTransfer(ctx, req)
	})
	if err != nil {
		return nil, err
	}
	return result.(*internal.InitiateResponse), nil
}

// ── Router ───────────────────────────────────────────────────────────

type PaymentRouter struct {
	providers []*ProviderBreaker
}

func NewPaymentRouter(providers []*ProviderBreaker) *PaymentRouter {
	return &PaymentRouter{providers: providers}
}

func (r *PaymentRouter) Route(ctx context.Context, req internal.InitiateRequest) (*internal.InitiateResponse, error) {
	var lastErr error
	for _, pb := range r.providers {
		resp, err := pb.InitiateTransfer(ctx, req)
		if err != nil {
			slog.Warn("provider failed, trying next",
				"provider", pb.Provider.Name(),
				"err", err,
			)
			lastErr = err
			continue
		}
		slog.Info("routed to provider", "provider", pb.Provider.Name())
		return resp, nil
	}
	return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

func (r *PaymentRouter) VerifySignature(providerName string, payload []byte, signature string) bool {
	for _, pb := range r.providers {
		if pb.Provider.Name() == providerName {
			// We don't wrap this in a circuit breaker because
			// signature verification is a local CPU operation, not a network call.
			return pb.Provider.VerifyWebhookSignature(payload, signature)
		}
	}
	return false
}

func (r *PaymentRouter) ProviderStatuses() map[string]string {
	statuses := make(map[string]string)
	for _, pb := range r.providers {
		statuses[pb.Provider.Name()] = pb.breaker.State().String()
	}
	return statuses
}

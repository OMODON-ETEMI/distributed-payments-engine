package routes

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
)

func TestHandlePaystackWebhook_SignatureAndRouting(t *testing.T) {
	p := routes.NewMockProvider("paystack", 0.0)
	pb := routes.NewProviderBreaker(p, routes.BreakerConfig{MaxRequests: 1, Interval: time.Second, Timeout: time.Second, ConsecutiveFailThreshold: 1})
	router := routes.NewPaymentRouter([]*routes.ProviderBreaker{pb})
	api := &routes.ApiConfig{Router: router}

	// invalid signature
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/webhook/paystack", bytes.NewBufferString(`{"event":"transfer.success","data":{}}`))
	req.Header.Set("X-Paystack-Signature", "bad")
	api.HandlePaystackWebhook(w, req)
	if w.Code != 401 {
		t.Fatalf("expected 401 for invalid signature got %d", w.Code)
	}

	// invalid json
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/webhook/paystack", bytes.NewBufferString("notjson"))
	req.Header.Set("X-Paystack-Signature", "mock-signature")
	api.HandlePaystackWebhook(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid json got %d", w.Code)
	}

	// unknown event -> acknowledged
	w = httptest.NewRecorder()
	body := map[string]interface{}{"event": "unknown.event", "data": map[string]interface{}{}}
	b, _ := json.Marshal(body)
	req = httptest.NewRequest("POST", "/v1/webhook/paystack", bytes.NewBuffer(b))
	req.Header.Set("X-Paystack-Signature", "mock-signature")
	api.HandlePaystackWebhook(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 for unknown event got %d", w.Code)
	}
}

package routes

import (
	"encoding/json"
	"io"
	"net/http"
)

type PaystackWebhookBody struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type PaystackTransferData struct {
	TransferCode  string `json:"transfer_code"`
	Reference     string `json:"reference"` // this is YOUR transfer_request_id
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason"`
}

func (api *ApiConfig) HandlePaystackWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 400, "cannot read body")
		return
	}

	// 1. Verify signature — ALWAYS do this first
	//    Paystack sends HMAC-SHA512 of the body signed with your secret key
	signature := r.Header.Get("X-Paystack-Signature")
	if !api.Router.VerifySignature("paystack", body, signature) {
		respondWithError(w, 401, "invalid signature")
		return
	}

	// 2. Parse event
	var event PaystackWebhookBody
	if err := json.Unmarshal(body, &event); err != nil {
		respondWithError(w, 400, "invalid json")
		return
	}

	// 3. Route by event type
	switch event.Event {
	case "transfer.success":
		api.handleTransferSuccess(w, r, event.Data)
	case "transfer.failed", "transfer.reversed":
		api.handleTransferFailed(w, r, event.Data)
	default:
		// Unknown event — acknowledge and ignore
		// IMPORTANT: always return 200 to Paystack even for unknown events.
		// If you return non-200, Paystack retries the webhook repeatedly.
		respondeWithJson(w, 200, map[string]string{"received": "true"})
	}
}

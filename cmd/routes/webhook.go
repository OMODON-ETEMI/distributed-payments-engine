package routes

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
)

// HandlePaystackWebhook receives and processes webhook events from Paystack.
// @Summary Paystack webhook endpoint
// @Description Receives webhook events from Paystack. Supported events: transfer.success, transfer.failed, transfer.reversed.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param X-Paystack-Signature header string true "HMAC-SHA512 signature"
// @Param body body WebhookBody true "Webhook Payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errResponse
// @Failure 401 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /webhook/paystack [post]
func (api *ApiConfig) HandlePaystackWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		internal.RespondWithError(w, 400, "cannot read body")
		return
	}

	// 1. Verify signature — ALWAYS do this first
	//    Paystack sends HMAC-SHA512 of the body signed with your secret key
	signature := r.Header.Get("X-Paystack-Signature")
	if !api.Router.VerifySignature("paystack", body, signature) {
		internal.RespondWithError(w, 401, "invalid signature")
		return
	}

	// 2. Minimal Metadata Extraction
	var event internal.WebhookBody
	if err := json.Unmarshal(body, &event); err != nil {
		internal.RespondWithError(w, 400, "invalid json")
		return
	}

	// 3. Kafka Handoff (The "Shock Absorber")
	topic := "withdrawal.webhook"
	if err := api.Kafka_producer.SendMessage(topic, event.ID, body); err != nil {
		log.Printf("Kafka Failure: %v", err)
		internal.RespondWithError(w, 500, "internal storage error")
		return
	}

	internal.RespondWithJson(w, 200, map[string]string{"received": "true"})
}

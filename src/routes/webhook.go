package routes

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

type WebhookBody struct {
	Event string          `json:"event"`
	ID    string          `json:"id"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data"`
}

type WebhookTransferData struct {
	ID            string          `json:"id"`
	Amount        string          `json:"Amount"`
	Currency      string          `json:"Currency"`
	Domain        string          `json:"Domain"`
	AccountNumber string          `json:"account_number"`
	BankCode      string          `json:"bank_code"`
	FullName      string          `json:"full_name"`
	Customer      json.RawMessage `json:"customer"`
	Reference     string          `json:"reference"`
	Status        string          `json:"status"`
	FailureReason string          `json:"failure_reason"`
}
type Customer struct {
	ID            string `json:"id"`
	AccountNumber string `json:"account_number"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
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
	var event WebhookBody
	if err := json.Unmarshal(body, &event); err != nil {
		respondWithError(w, 400, "invalid json")
		return
	}

	_, err = api.Db.Queries.CreateIncomingWebhook(r.Context(), db.CreateIncomingWebhookParams{
		Provider:        "paystack",
		ExternalEventID: pgtype.Text{String: event.ID, Valid: event.ID != ""},
		EventType:       pgtype.Text{String: event.Type, Valid: event.Type != ""},
		Payload:         body,
	})
	if err != nil {
		respondWithError(w, 500, "cannot create webhook")
		return
	}
	respondeWithJson(w, 200, map[string]string{"received": "true"})

}

func (api *ApiConfig) HandleWebhookLogic(ctx context.Context, data WebhookBody, webhook db.IncomingWebhook) {
	var err error
	switch data.Event {
	case "transfer.success":
		err = api.handleTransferSuccess(ctx, nil, data.Data)
	case "transfer.failed", "transfer.reversed":
		api.handleTransferFailed(ctx, nil, data.Data)
	default:
		// Unknown event — acknowledge and ignore"
	}
	if err != nil {
		log.Printf("Worker failed to process webhook %s: %v", data.ID, err)
		_, err := api.Db.Queries.UpdateIncomingWebhookStatus(ctx, db.UpdateIncomingWebhookStatusParams{
			Status:       "failed",
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			ID:           webhook.ID,
		})
		if err != nil {
			log.Printf("Worker failed to update webhook status: %v", err)
		}
		return
	}
	_, err = api.Db.Queries.UpdateIncomingWebhookStatus(ctx, db.UpdateIncomingWebhookStatusParams{
		Status: "success",
		ID:     webhook.ID,
	})
}

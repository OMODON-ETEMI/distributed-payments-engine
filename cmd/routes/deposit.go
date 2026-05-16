package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
)

// HandleDeposit credits funds to an account from the system settlement account.
// @Summary Deposit funds to account
// @Description Credits funds to an account from the system settlement account. Supports idempotent deposits.
// @Tags Deposits
// @Accept json
// @Produce json
// @Param body body DepositParams true "Deposit Details"
// @Success 202 {object} map[string]interface{}{"success":true,"status":"pending","reference":"DEP-{idempotency_key_id}","message":"Transfer is being processed. You will be notified via webhook."}
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /account/deposit [post]
func (api *ApiConfig) HandleDeposit(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		internal.RespondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := internal.DepositParams{}
	if err := decoder.Decode(&params); err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.Provider == "" || params.IdempotencyKeyID == "" || params.CustomerID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Sourcesystem == "" || params.Amount == "" || params.FeeAmount == "" || params.ClientReference == "" || params.ExternalReference == "" {
		internal.RespondWithError(w, 400, "missing required fields: provider, idempotency_key_id, customer_id, destination_account_id, currency_code, source_system, amount, fee_amount, client_reference, external_reference")
		return
	}
	if params.Metadata == nil {
		params.Metadata = make(map[string]interface{})
	}
	payload, err := json.Marshal(params)
	if err != nil {
		internal.RespondWithError(w, 500, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	topic := "deposit.transfer"
	if err := api.Kafka_producer.SendMessage(topic, params.CustomerID, payload); err != nil {
		log.Printf("Kafka Failure: %v", err)
		internal.RespondWithError(w, 500, "internal storage error")
		return
	}
	internal.RespondWithJson(w, 202, map[string]interface{}{
		"success":   true,
		"status":    "pending",
		"reference": fmt.Sprintf("DEP-%s", params.IdempotencyKeyID),
		"message":   "Transfer is being processed. You will be notified via webhook.",
	})
}

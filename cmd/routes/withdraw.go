package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/services"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
)

// HandleWithdraw debits funds from an account to the system settlement account.
// @Summary Withdraw funds from account
// @Description Debits funds from an account to the system settlement account. Supports idempotent withdrawals.
// @Tags Withdrawals
// @Accept json
// @Produce json
// @Param body body internal.WithdrawParams true "Withdrawal Details"
// @Success 201 {object} TransferResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /account/withdraw [post]
func (api *ApiConfig) HandleWithdraw(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		internal.RespondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := internal.WithdrawParams{}
	if err := decoder.Decode(&params); err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Sourcesystem == "" || params.Amount == "" || params.FeeAmount == "" || params.ClientReference == "" || params.ExternalReference == "" {
		internal.RespondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, destination_account_id, currency_code, source_system, amount, fee_amount, client_reference, external_reference")
		return
	}

	resp, err := services.WithdrawalLogic(r.Context(), bodyBytes, api.Db, api.Redis, api.Router)
	if err != nil {
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 201, resp)
}

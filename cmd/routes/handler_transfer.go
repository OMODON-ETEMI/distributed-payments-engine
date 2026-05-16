package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/services"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/utilities"
	"github.com/go-chi/chi"
	"github.com/jackc/pgx/v5"
)

type GetTransferByIDParams struct {
	TransferID string `json:"transfer_id"`
}

// HandleCreateTransfer creates a money transfer from one account to another.
// @Summary Create a transfer between accounts
// @Description Creates a money transfer from one account to another with automatic fee collection. Supports idempotent transfers.
// @Tags Transfers
// @Accept json
// @Produce json
// @Param body body TransferParams true "Transfer Details"
// @Success 201 {object} TransferResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /account/transfer [post]
func (api *ApiConfig) HandleCreateTransfer(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		internal.RespondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}
	requestHash := internal.HashRequest(bodyBytes)

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := internal.TransferParams{}
	if err := decoder.Decode(&params); err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.SourceAccountID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Description == "" {
		internal.RespondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, description")
		return
	}

	resp, err := services.CreateTransfer(r.Context(), params, api.Db, api.Redis, requestHash)
	if err != nil {
		internal.RespondWithError(w, 400, err.Error())
		return
	}

	internal.RespondWithJson(w, 201, resp)
}

// GetTransferbyID retrieves details of a specific transfer transaction.
// @Summary Get transfer by ID
// @Description Retrieves details of a specific transfer transaction.
// @Tags Transfers
// @Produce json
// @Param id path string true "Transfer UUID" format(uuid)
// @Success 201 {object} TransferResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /transfer/{id} [get]
func (api *ApiConfig) GetTransferbyID(w http.ResponseWriter, r *http.Request) {
	transferIDStr := chi.URLParam(r, "id")
	if transferIDStr == "" {
		internal.RespondWithError(w, 400, "Transfer ID is required")
		return
	}

	resp, err := services.GetTransferByID(r.Context(), transferIDStr, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "transfer not found")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 201, resp)
}

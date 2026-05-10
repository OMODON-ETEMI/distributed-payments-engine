package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/go-chi/chi"
	"github.com/jackc/pgx/v5"
)

type HoldParams struct {
	ID                string  `json:"id"`
	TransferRequestID string  `json:"transfer_id"`
	Amount            string  `json:"amount"` // original hold amount
	ReasonCode        *string `json:"reason_code,omitempty"`
}

// ConsumeHold converts a held amount to an actual debit and releases remaining funds.
// @Summary Consume a hold
// @Description Converts a held amount to an actual debit and releases remaining funds.
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body HoldParams true "Hold Consumption Details"
// @Success 200 {object} object
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /admin/hold/consume [post]
func (api *ApiConfig) ConsumeHold(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := HoldParams{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing json: %v", err))
	}
	amount, err := StringToNumeric(params.Amount)
	if err != nil {
		respondWithError(w, 400, "invalid amount")
		return
	}
	id, err := StringtoPgUuid(params.ID)
	if err != nil {
		respondWithError(w, 400, "Error parsing ID")
		return
	}
	_, err = api.Db.Queries.ConsumeHold(r.Context(), db.ConsumeHoldParams{
		Amount: amount,
		ID:     id,
	})
	_, err = api.Db.Queries.ReleaseHold(r.Context(), db.ReleaseHoldParams{
		Amount: amount,
		ID:     id,
	})
	respondeWithJson(w, 200, struct{}{})
}

// GetHoldBytransferRequest retrieves the active hold placed on a transfer.
// @Summary Get active hold for transfer
// @Description Retrieves the active hold placed on a transfer for payment processing.
// @Tags Admin
// @Produce json
// @Param transfer_id path string true "Transfer request UUID" format(uuid)
// @Success 200 {object} HoldResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /admin/hold/{transfer_id} [get]
func (api *ApiConfig) GetHoldBytransferRequest(w http.ResponseWriter, r *http.Request) {
	TransferRequestID := chi.URLParam(r, "transfer_id")
	if TransferRequestID == "" {
		respondWithError(w, 404, "Transfer Request ID is needed ")
		return
	}
	trfId, err := StringtoPgUuid(TransferRequestID)
	if err != nil {
		respondWithError(w, 400, "Error parsing ID")
		return
	}
	hold, err := api.Db.Queries.GetActiveHoldByTransferRequestID(r.Context(), trfId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "No Active Hold Exist on this Transaction")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error gettig the the Hold: %v", err))
		return
	}
	respondeWithJson(w, 200, ToHoldResponse(hold))
}

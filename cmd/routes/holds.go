package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/services"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/go-chi/chi"
	"github.com/jackc/pgx/v5"
)

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
	params := internal.HoldParams{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing json: %v", err))
		return
	}

	err = services.ConsumeHold(r.Context(), params, api.Db.Queries)
	if err != nil {
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 200, struct{}{})
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
		internal.RespondWithError(w, 404, "Transfer Request ID is needed")
		return
	}

	hold, err := services.GetHoldByTransferRequest(r.Context(), TransferRequestID, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "No Active Hold Exist on this Transaction")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}
	internal.RespondWithJson(w, 200, hold)
}

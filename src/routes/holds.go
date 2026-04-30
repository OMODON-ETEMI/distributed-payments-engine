package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5"
)

type HoldParams struct {
	ID                string  `json:"id"`
	TransferRequestID string  `json:"transfer_id"`
	Amount            string  `json:"amount"` // original hold amount
	ReasonCode        *string `json:"reason_code,omitempty"`
}

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
	}
	id, err := StringtoPgUuid(params.ID)
	if err != nil {
		respondWithError(w, 400, "Error parsing ID")
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

func (api *ApiConfig) GetHoldBytransferRequest(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := HoldParams{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing json: %v", err))
	}
	if params.TransferRequestID == "" {
		respondWithError(w, 404, "Transfer Request ID is needed ")
		return
	}
	trfId, err := StringtoPgUuid(params.TransferRequestID)
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

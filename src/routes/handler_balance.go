package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type BalanceParameters struct {
	AccountID string `json:"account_id"`
}

func (api *ApiConfig) HandleGetBalancesForAccount(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := BalanceParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.AccountID == "" {
		respondWithError(w, 400, "account_id is required")
		return
	}
	id, err := StringtoPgUuid(params.AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// ensure account exists
	_, err = api.Db.Queries.GetAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	balances, err := api.Db.Queries.GetBalancesForAccount(r.Context(), id)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error getting balances for account: %v", err))
		return
	}
	out := make([]BalanceProjectionResponse, 0, len(balances))
	for _, b := range balances {
		out = append(out, GetBalancesForAccountRowToResponse(b))
	}
	respondeWithJson(w, 200, out)
}

// func (api *ApiConfig) HandleRebuildBalanceProjection(w http.ResponseWriter, r *http.Request) {
// 	decoder := json.NewDecoder(r.Body)
// 	params := BalanceParameters{}
// 	err := decoder.Decode(&params)
// 	if err != nil {
// 		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
// 		return
// 	}
// 	if params.AccountID == "" || params.CurrencyCode == "" {
// 		respondWithError(w, 400, "missing required fields: account_id or currency_code")
// 		return
// 	}
// 	id, err := StringtoPgUuid(params.AccountID)
// 	if err != nil {
// 		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
// 		return
// 	}

// 	// ensure account exists
// 	_, err = api.Db.GetAccountByID(r.Context(), id)
// 	if err != nil {
// 		if errors.Is(err, pgx.ErrNoRows) {
// 			respondWithError(w, 404, "account not found")
// 			return
// 		}
// 		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
// 		return
// 	}

// 	err = api.Db.RebuildBalanceProjection(r.Context(), database.RebuildBalanceProjectionParams{AccountID: id, CurrencyCode: params.CurrencyCode})
// 	if err != nil {
// 		respondWithError(w, 500, fmt.Sprintf("Error rebuilding balance projection: %v", err))
// 		return
// 	}
// 	respondeWithJson(w, 200, struct{}{})
// }

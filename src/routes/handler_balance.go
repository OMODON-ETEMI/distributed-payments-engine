package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5"
)

type BalanceParameters struct {
	ID               string `json:"id"`
	AccountID        string `json:"account_id"`
	ExternalRef      string `json:"external_ref"`
	AccountNumber    string `json:"account_number"`
	AccountType      string `json:"account_type"`
	Status           string `json:"status"`
	CurrencyCode     string `json:"currency_code"`
	BalanceKind      string `json:"balance_kind"`
	LedgerNormalSide string `json:"ledger_normal_side"`
	Limit            int    `json:"limit"`
	Offset           int    `json:"offset"`
}

func (api *ApiConfig) HandleComputeHeldAmount(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := BalanceParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.AccountID == "" || params.CurrencyCode == "" {
		respondWithError(w, 400, "missing required fields: account_id or currency_code")
		return
	}

	id, err := StringtoPgUuid(params.AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// ensure account exists
	_, err = api.Db.GetAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	balance, err := api.Db.ComputeHeldAmount(r.Context(), database.ComputeHeldAmountParams{
		AccountID:    id,
		CurrencyCode: params.CurrencyCode,
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error computing Held Amount: %v", err))
		return
	}
	respondeWithJson(w, 200, HeldAmountResponse{HeldBalance: formatNumeric(balance)})
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
	_, err = api.Db.GetAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	balances, err := api.Db.GetBalancesForAccount(r.Context(), id)
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

func (api *ApiConfig) HandleGetBalanceProjection(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := BalanceParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.AccountID == "" || params.CurrencyCode == "" || params.BalanceKind == "" {
		respondWithError(w, 400, "missing required fields: account_id, currency_code, balance_kind")
		return
	}
	id, err := StringtoPgUuid(params.AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// ensure account exists
	_, err = api.Db.GetAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	projection, err := api.Db.GetBalanceProjection(r.Context(), database.GetBalanceProjectionParams{
		AccountID:    id,
		CurrencyCode: params.CurrencyCode,
		BalanceKind:  params.BalanceKind,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "balance projection not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error getting balance projection: %v", err))
		return
	}
	respondeWithJson(w, 200, BalanceProjectionResponseObject(projection))
}

func (api *ApiConfig) HandleComputeLedgerBalance(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := BalanceParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.AccountID == "" || params.CurrencyCode == "" {
		respondWithError(w, 400, "missing required fields: account_id or currency_code")
		return
	}
	id, err := StringtoPgUuid(params.AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// ensure account exists
	_, err = api.Db.GetAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	row, err := api.Db.ComputeLedgerBalance(r.Context(), database.ComputeLedgerBalanceParams{AccountID: id, CurrencyCode: params.CurrencyCode})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error computing ledger balance: %v", err))
		return
	}
	respondeWithJson(w, 200, ComputeLedgerBalanceRowToResponse(row))
}

func (api *ApiConfig) HandleRebuildBalanceProjection(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := BalanceParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.AccountID == "" || params.CurrencyCode == "" {
		respondWithError(w, 400, "missing required fields: account_id or currency_code")
		return
	}
	id, err := StringtoPgUuid(params.AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// ensure account exists
	_, err = api.Db.GetAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	err = api.Db.RebuildBalanceProjection(r.Context(), database.RebuildBalanceProjectionParams{AccountID: id, CurrencyCode: params.CurrencyCode})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error rebuilding balance projection: %v", err))
		return
	}
	respondeWithJson(w, 200, struct{}{})
}

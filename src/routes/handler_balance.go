package routes

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/jackc/pgx/v5"
)

type BalanceParameters struct {
	AccountID string `json:"account_id"`
}

// HandleGetBalancesForAccount retrieves all balance information for an account.
// @Summary Get balances for account
// @Description Retrieves all balance information for an account including ledger, available, and held amounts.
// @Tags Balances
// @Produce json
// @Param id path string true "Account UUID" format(uuid)
// @Success 200 {array} BalanceResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /account/{id}/balances [get]
func (api *ApiConfig) HandleGetBalancesForAccount(w http.ResponseWriter, r *http.Request) {
	AccountID := chi.URLParam(r, "id")
	if AccountID == "" {
		respondWithError(w, 400, "account_id is required")
		return
	}
	id, err := StringtoPgUuid(AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// ensure account exists
	Acct, err := api.Db.Queries.GetAccountByID(r.Context(), id)
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
	out := make([]BalanceResponse, 0, len(balances))
	for _, b := range balances {
		out = append(out, ToBalanceResponse(AccountID, Acct.CurrencyCode, b))
	}
	respondeWithJson(w, 200, out)
}

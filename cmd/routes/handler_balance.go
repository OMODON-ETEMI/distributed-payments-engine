package routes

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/services"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
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
		internal.RespondWithError(w, 400, "account_id is required")
		return
	}

	// ensure account exists
	Acct, err := services.GetAccountByID(r.Context(), AccountID, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "account not found")
			return
		}
		internal.RespondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}

	balances, err := services.GetBalance(r.Context(), *Acct, api.Db.Queries)
	if err != nil {
		internal.RespondWithError(w, 500, fmt.Sprintf("Error looking up balance for Account ID:%w %v", Acct.ID, err))
		return
	}

	internal.RespondWithJson(w, 200, balances)
}

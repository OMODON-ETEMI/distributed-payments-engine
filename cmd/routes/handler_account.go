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

// HandleCreateAccount creates a new account for a customer.
// @Summary Create a new account
// @Description Creates a new account for a customer. Idempotent by external_ref and account_number.
// @Tags Accounts
// @Accept json
// @Produce json
// @Param body body AccountParameters true "Account Creation Details"
// @Success 200 {object} AccountResponse
// @Failure 400 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /create/account [post]
func (api *ApiConfig) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.ExternalRef == "" || params.AccountNumber == "" || params.CustomerID == "" || params.AccountType == "" || params.CurrencyCode == "" || params.LedgerNormalSide == "" {
		internal.RespondWithError(w, 400, "missing required fields: external_ref, account_number, customer_id, account_type, currency_code, ledger_normal_side")
		return
	}

	account, err := services.CreateAccount(r.Context(), params, api.Db, api.Db.Queries)
	if err != nil {
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 200, account)
}

// HandleGetAccountByAccountNumber retrieves account details by its account number.
// @Summary Get account by account number
// @Description Retrieves account details by its account number.
// @Tags Accounts
// @Produce json
// @Param number path string true "Account number"
// @Success 200 {object} AccountResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /account/number/{number} [get]
func (api *ApiConfig) HandleGetAccountByAccountNumber(w http.ResponseWriter, r *http.Request) {
	AccountNumber := chi.URLParam(r, "number")
	if AccountNumber == "" {
		internal.RespondWithError(w, 400, "Account Number is required")
		return
	}
	account, err := services.GetAccountByAccountNumber(r.Context(), AccountNumber, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "Account not found")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 200, account)
}

// HandleListAccountByCustomer returns paginated list of accounts owned by a specific customer.
// @Summary List accounts for a customer
// @Description Returns paginated list of accounts owned by a specific customer.
// @Tags Accounts
// @Accept json
// @Produce json
// @Param body body AccountParameters true "Customer ID and Pagination details"
// @Success 200 {array} AccountResponse
// @Failure 400 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /list/accounts/customer [post]
func (api *ApiConfig) HandleListAccountByCustomer(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.CustomerID == "" {
		internal.RespondWithError(w, 400, "Customer ID is required")
		return
	}

	accounts, err := services.ListAccountsByCustomer(r.Context(), params, api.Db.Queries)
	if err != nil {
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 200, accounts)
}

// HandleUpdateAccountStatus updates the status of an existing account.
// @Summary Update account status
// @Description Updates the status of an existing account (e.g., active, suspended, closed).
// @Tags Accounts
// @Accept json
// @Produce json
// @Param body body AccountParameters true "Account ID and New Status"
// @Success 200 {object} AccountResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /update/account [post]
func (api *ApiConfig) HandleUpdateAccountStatus(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.CustomerID == "" {
		internal.RespondWithError(w, 400, "Account ID is required")
		return
	}
	if params.Status == "" {
		internal.RespondWithError(w, 400, "Status is required")
		return
	}
	account, err := services.UpdateAccountStatus(r.Context(), params, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "Account not found")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}
	internal.RespondWithJson(w, 200, account)
}

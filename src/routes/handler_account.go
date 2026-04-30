package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AccountParameters struct {
	ID               string                 `json:"id"`
	CustomerID       string                 `json:"customer_id"`
	ExternalRef      string                 `json:"external_ref"`
	AccountNumber    string                 `json:"account_number"`
	AccountType      string                 `json:"account_type"`
	Status           string                 `json:"status"`
	Metadata         map[string]interface{} `json:"metadata"`
	CurrencyCode     string                 `json:"currency_code"`
	LedgerNormalSide string                 `json:"ledger_normal_side"`
	Limit            int                    `json:"limit"`
	Offset           int                    `json:"offset"`
}

func (api *ApiConfig) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.ExternalRef == "" || params.AccountNumber == "" || params.CustomerID == "" || params.AccountType == "" || params.CurrencyCode == "" {
		respondWithError(w, 400, "missing required fields: external_ref, account_number, customer_id, account_type, currency_code")
		return
	}
	metadataBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}
	customer_id, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	// idempotency: prefer existing by external_ref, fallback to account_number
	if params.ExternalRef != "" {
		existing, err := api.Db.Queries.GetAccountByExternalRef(r.Context(), params.ExternalRef)
		if err == nil {
			respondeWithJson(w, 200, AccountResponseObject(existing))
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 500, fmt.Sprintf("Error checking existing account: %v", err))
			return
		}
	}
	if params.AccountNumber != "" {
		existing, err := api.Db.Queries.GetAccountByNumber(r.Context(), params.AccountNumber)
		if err == nil {
			respondeWithJson(w, 200, AccountResponseObject(existing))
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 500, fmt.Sprintf("Error checking existing account by number: %v", err))
			return
		}
	}

	openedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	account, err := api.Db.Queries.CreateAccount(r.Context(), database.CreateAccountParams{
		CustomerID:       customer_id,
		ExternalRef:      params.ExternalRef,
		AccountNumber:    params.AccountNumber,
		AccountType:      params.AccountType,
		Status:           params.Status,
		CurrencyCode:     params.CurrencyCode,
		LedgerNormalSide: params.LedgerNormalSide,
		Metadata:         metadataBytes,
		OpenedAt:         openedAt,
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating Account: %v", err))
		return
	}
	respondeWithJson(w, 200, AccountResponseObject(account))
}

// func (api *ApiConfig) HandleGetAccountByExternalRef(w http.ResponseWriter, r *http.Request) {
// 	decoder := json.NewDecoder(r.Body)
// 	params := AccountParameters{}
// 	err := decoder.Decode(&params)
// 	if err != nil {
// 		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
// 		return
// 	}
// 	if params.ExternalRef == "" {
// 		respondWithError(w, 400, "external_ref is required")
// 		return
// 	}

// 	account, err := api.Db.Queries.GetAccountByExternalRef(r.Context(), params.ExternalRef)
// 	if err != nil {
// 		if errors.Is(err, pgx.ErrNoRows) {
// 			respondWithError(w, 404, "Account not found")
// 			return
// 		}
// 		respondWithError(w, 500, fmt.Sprintf("Error getting Account by External Ref: %v", err))
// 		return
// 	}

// 	respondeWithJson(w, 200, AccountResponseObject(account))
// }

// func (api *ApiConfig) HandleGetAccountByID(w http.ResponseWriter, r *http.Request) {
// 	decoder := json.NewDecoder(r.Body)
// 	params := AccountParameters{}
// 	err := decoder.Decode(&params)
// 	if err != nil {
// 		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
// 		return
// 	}
// 	if params.CustomerID == "" {
// 		respondWithError(w, 400, "customer ID is required")
// 		return
// 	}

// 	id, err := StringtoPgUuid(params.CustomerID)
// 	if err != nil {
// 		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
// 		return
// 	}

// 	account, err := api.Db.Queries.GetAccountByID(r.Context(), id)
// 	if err != nil {
// 		if errors.Is(err, pgx.ErrNoRows) {
// 			respondWithError(w, 404, "Account not found")
// 			return
// 		}
// 		respondWithError(w, 500, fmt.Sprintf("Error getting Account by ID: %v", err))
// 		return
// 	}
// 	respondeWithJson(w, 200, AccountResponseObject(account))
// }

func (api *ApiConfig) HandleGetAccountByAccountNumber(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.AccountNumber == "" {
		respondWithError(w, 400, "Account Number is required")
		return
	}

	account, err := api.Db.Queries.GetAccountByNumber(r.Context(), params.AccountNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "Account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error getting Account by Account Number: %v", err))
		return
	}
	respondeWithJson(w, 200, AccountResponseObject(account))
}

func (api *ApiConfig) HandleListAccountByCustomer(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}

	limit := int32(50)
	offset := int32(0)
	if params.Limit > 0 {
		limit = int32(params.Limit)
	}
	if params.Offset > 0 {
		offset = int32(params.Offset)
	}
	if limit > 1000 {
		limit = 1000
	}
	if params.CustomerID == "" {
		respondWithError(w, 400, "Customer ID is required")
		return
	}
	id, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}

	account, err := api.Db.Queries.ListAccountsByCustomer(r.Context(), database.ListAccountsByCustomerParams{
		Limit:      limit,
		Offset:     offset,
		CustomerID: id,
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error Listing Account: %v", err))
		return
	}
	var accounts []AccountResponse

	for _, a := range account {
		accounts = append(accounts, AccountResponseObject(a))
	}
	respondeWithJson(w, 200, accounts)
}

func (api *ApiConfig) HandleUpdateAccountStatus(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := AccountParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.Status == "" {
		respondWithError(w, 400, "A New Status is required")
		return
	}
	id, err := StringtoPgUuid(params.ID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}
	// set closed_at when status indicates closure
	var closedAt pgtype.Timestamptz
	if params.Status == "closed" {
		closedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	} else {
		closedAt = pgtype.Timestamptz{Valid: false}
	}

	account, err := api.Db.Queries.UpdateAccountStatus(r.Context(), database.UpdateAccountStatusParams{
		Status:   params.Status,
		ClosedAt: closedAt,
		ID:       id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "Account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error Updating Account: %v", err))
		return
	}
	respondeWithJson(w, 200, AccountResponseObject(account))
}

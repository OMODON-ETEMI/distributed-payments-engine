package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type JournalParameters struct {
	ID                   string                 `json:"id"`
	AccountID            string                 `json:"account_id"`
	TransactionRef       string                 `json:"transaction_ref"`
	TransferRequestID    string                 `json:"transfer_request_id"`
	IdempotencyKeyID     string                 `json:"idempotency_key_id"`
	Status               string                 `json:"status"`
	EntryType            string                 `json:"entry_type"`
	AccountingDate       string                 `json:"accounting_date"`
	EffectiveAt          string                 `json:"effective_at"`
	SourceSystem         string                 `json:"source_system"`
	SourceEventID        string                 `json:"source_event_id"`
	Description          string                 `json:"description"`
	Metadata             map[string]interface{} `json:"metadata"`
	JournalTransactionID string                 `json:"journal_transaction_id"`
	LineNumber           int32                  `json:"line_number"`
	Side                 string                 `json:"side"`
	Amount               string                 `json:"amount"`
	CurrencyCode         string                 `json:"currency_code"`
	BalanceKind          string                 `json:"balance_kind"`
	Memo                 string                 `json:"memo"`
}

func (api *ApiConfig) HandleCreateJournalLine(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := JournalParameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}

	// required fields
	if params.JournalTransactionID == "" || params.AccountID == "" || params.LineNumber == 0 || params.Side == "" || params.Amount == "" || params.CurrencyCode == "" {
		respondWithError(w, 400, "missing required fields: journal_transaction_id, account_id, line_number, side, amount, currency_code")
		return
	}

	// currency code sanity check (DB enforces too)
	currencyRe := regexp.MustCompile(`^[A-Z]{3}$`)
	if !currencyRe.MatchString(params.CurrencyCode) {
		respondWithError(w, 400, "invalid currency_code, expected 3 uppercase letters")
		return
	}

	// balance kind check
	allowedKinds := map[string]bool{"ledger": true, "available": true, "held": true}
	if params.BalanceKind == "" {
		params.BalanceKind = "ledger"
	}
	if !allowedKinds[params.BalanceKind] {
		respondWithError(w, 400, "invalid balance_kind, must be one of: ledger, available, held")
		return
	}

	// side check
	if params.Side != "debit" && params.Side != "credit" {
		respondWithError(w, 400, "side must be either 'debit' or 'credit'")
		return
	}

	// parse transaction id and ensure exists
	txID, err := StringtoPgUuid(params.JournalTransactionID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing journal_transaction_id: %v", err))
		return
	}
	jt, err := api.Db.GetJournalTransactionByID(r.Context(), txID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "journal transaction not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up journal transaction: %v", err))
		return
	}
	if jt.Status == "posted" {
		respondWithError(w, 400, "cannot add lines to a posted journal transaction")
		return
	}

	// parse account and ensure exists
	acctID, err := StringtoPgUuid(params.AccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing account_id: %v", err))
		return
	}
	acct, err := api.Db.GetAccountByID(r.Context(), acctID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up account: %v", err))
		return
	}
	// ensure currency matches account currency to avoid cross-currency journal lines
	if acct.CurrencyCode != params.CurrencyCode {
		respondWithError(w, 400, "currency_code must match account currency")
		return
	}

	// ensure line number not duplicated for the transaction
	existingLines, err := api.Db.ListJournalLinesForTransaction(r.Context(), txID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error listing existing journal lines: %v", err))
		return
	}
	for _, l := range existingLines {
		if l.LineNumber == params.LineNumber {
			respondWithError(w, 409, "line number already exists for this journal transaction")
			return
		}
	}

	// parse amount into pgtype.Numeric
	var amount pgtype.Numeric
	if err := amount.Scan(params.Amount); err != nil {
		respondWithError(w, 400, fmt.Sprintf("invalid amount: %v", err))
		return
	}

	// marshal metadata
	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
			return
		}
		metaBytes = b
	}

	// prepare memo
	memo := pgtype.Text{String: params.Memo, Valid: params.Memo != ""}

	created, err := api.Db.CreateJournalLine(r.Context(), db.CreateJournalLineParams{
		JournalTransactionID: txID,
		LineNumber:           params.LineNumber,
		AccountID:            acctID,
		Side:                 params.Side,
		Amount:               amount,
		CurrencyCode:         params.CurrencyCode,
		BalanceKind:          params.BalanceKind,
		Memo:                 memo,
		Metadata:             metaBytes,
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating journal line: %v", err))
		return
	}
	respondeWithJson(w, 201, created)
}

func (api *ApiConfig) HandleCreateJournalTransaction(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := JournalParameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.TransactionRef == "" {
		respondWithError(w, 400, "transaction_ref is required")
		return
	}

	var transferReqID pgtype.UUID
	if params.TransferRequestID != "" {
		id, err := StringtoPgUuid(params.TransferRequestID)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("Error parsing transfer_request_id: %v", err))
			return
		}
		transferReqID = id
	}
	var idempKeyID pgtype.UUID
	if params.IdempotencyKeyID != "" {
		idk, err := StringtoPgUuid(params.IdempotencyKeyID)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("Error parsing idempotency_key_id: %v", err))
			return
		}
		idempKeyID = idk
	}

	var accDate pgtype.Date
	if params.AccountingDate != "" {
		d, err := time.Parse("2006-01-02", params.AccountingDate)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("invalid accounting_date: %v", err))
			return
		}
		accDate = pgtype.Date{Time: d, Valid: true}
	}

	// effective at
	var effAt pgtype.Timestamptz
	if params.EffectiveAt != "" {
		t, err := time.Parse(time.RFC3339, params.EffectiveAt)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("invalid effective_at: %v", err))
			return
		}
		effAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	srcEvent := pgtype.Text{String: params.SourceEventID, Valid: params.SourceEventID != ""}
	desc := pgtype.Text{String: params.Description, Valid: params.Description != ""}

	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
			return
		}
		metaBytes = b
	}

	created, err := api.Db.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
		TransactionRef:    params.TransactionRef,
		TransferRequestID: transferReqID,
		IdempotencyKeyID:  idempKeyID,
		Status:            params.Status,
		EntryType:         params.EntryType,
		AccountingDate:    accDate,
		EffectiveAt:       effAt,
		SourceSystem:      params.SourceSystem,
		SourceEventID:     srcEvent,
		Description:       desc,
		Metadata:          metaBytes,
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating journal transaction: %v", err))
		return
	}
	respondeWithJson(w, 201, JournalTransactionResponseObject(created))
}

func (api *ApiConfig) HandleGetJournalTransactionByRef(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var payload struct {
		TransactionRef string `json:"transaction_ref"`
	}
	if err := decoder.Decode(&payload); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if payload.TransactionRef == "" {
		respondWithError(w, 400, "transaction_ref is required")
		return
	}
	jt, err := api.Db.GetJournalTransactionByRef(r.Context(), payload.TransactionRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "journal transaction not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error getting journal transaction: %v", err))
		return
	}
	respondeWithJson(w, 200, JournalTransactionResponseObject(jt))
}

func (api *ApiConfig) HandleListJournalLinesForTransaction(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var payload struct {
		JournalTransactionID string `json:"journal_transaction_id"`
	}
	if err := decoder.Decode(&payload); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if payload.JournalTransactionID == "" {
		respondWithError(w, 400, "journal_transaction_id is required")
		return
	}
	txID, err := StringtoPgUuid(payload.JournalTransactionID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing journal_transaction_id: %v", err))
		return
	}
	lines, err := api.Db.ListJournalLinesForTransaction(r.Context(), txID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error listing journal lines: %v", err))
		return
	}
	out := make([]JournalLineResponse, 0, len(lines))
	for _, l := range lines {
		out = append(out, JournalLineResponseObject(l))
	}
	respondeWithJson(w, 200, out)
}

func (api *ApiConfig) HandleMarkJournalTransactionPosted(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var payload struct {
		ID string `json:"id"`
	}
	if err := decoder.Decode(&payload); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if payload.ID == "" {
		respondWithError(w, 400, "id is required")
		return
	}
	id, err := StringtoPgUuid(payload.ID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}
	jt, err := api.Db.MarkJournalTransactionPosted(r.Context(), id)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error marking journal transaction posted: %v", err))
		return
	}
	respondeWithJson(w, 200, JournalTransactionResponseObject(jt))
}

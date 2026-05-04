package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type DepositeParams struct {
	IdempotencyKeyID     string                 `json:"idempotency_key_id"`
	CustomerID           string                 `json:"customer_id"`
	SourceAccountID      string                 `json:"source_account_id"`
	DestinationAccountID string                 `json:"destination_account_id"`
	CurrencyCode         string                 `json:"currency_code"`
	Sourcesystem         string                 `json:"source_system"`
	Description          string                 `json:"description"`
	Amount               string                 `json:"amount"`
	FeeAmount            string                 `json:"fee_amount"`
	ClientReference      string                 `json:"client_reference"`
	ExternalReference    string                 `json:"external_reference"`
	Memo                 string                 `json:"memo"`
	Metadata             map[string]interface{} `json:"metadata"`
}

func (api *ApiConfig) HandleDeposite(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}
	requestHash := HashRequest(bodyBytes)

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := DepositeParams{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" {
		respondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, destination_account_id, currency_code")
		return
	}
	if params.Metadata == nil {
		params.Metadata = make(map[string]interface{})
	}
	customerID, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing customer ID: %v", err))
		return
	}
	customer, err := api.Db.Queries.GetCustomerByID(r.Context(), customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "customer not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up customer: %v", err))
		return
	}
	systemAcct, err := api.Db.Queries.GetAccountByExternalRef(r.Context(), "system_settlement_ngn")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "system settlement account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up system settlement account: %v", err))
		return
	}
	DestinationAccountID, err := StringtoPgUuid(params.DestinationAccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Destination account ID: %v", err))
		return
	}
	DestinationAccount, err := api.Db.Queries.GetAccountByID(r.Context(), DestinationAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "destination account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up destination account: %v", err))
		return
	}
	if DestinationAccount.Status != "active" {
		respondWithError(w, 400, "destination account is not active")
		return
	}
	if params.CurrencyCode != DestinationAccount.CurrencyCode {
		respondWithError(w, 400, "currency code mismatch")
		return
	}
	amount, err := StringToNumeric(params.Amount)
	feeAmount, err := StringToNumeric(params.FeeAmount)
	ClientReference := pgtype.Text{String: params.ClientReference, Valid: params.ClientReference != ""}
	ExternalReference := pgtype.Text{String: params.ExternalReference, Valid: params.ExternalReference != ""}
	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
			return
		}
		metaBytes = b
	}
	memo := pgtype.Text{String: params.Memo, Valid: params.Memo != ""}
	check, err := api.IdemCheck(r.Context(), params.IdempotencyKeyID, params.CustomerID, requestHash, "deposit_create")
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error checking idempotency key: %v", err))
		return
	}
	if !check.ShouldProceed {
		var cachedData TransferResponse
		err := json.Unmarshal(check.CachedResponse, &cachedData)
		if err != nil {
			fmt.Printf("Failed to unmarshal cached response: %v", err)
			respondeWithJson(w, check.StatusCode, string(check.CachedResponse))
			return
		}
		respondeWithJson(w, check.StatusCode, cachedData)
		return
	}
	idempkey, err := api.Db.Queries.CreateIdempotencyKey(r.Context(), db.CreateIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKeyID,
		Scope:          "deposit_create",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating idempotency key: %v", err))
		return
	}

	var trf db.TransferRequest
	var jtx db.JournalTransaction
	var payloadBytes []byte
	err = api.Db.ExecTx(r.Context(), func(q *db.Queries) error {
		systemSettleAcct, err := q.GetAccountByIDForUpdate(r.Context(), systemAcct.ID)
		if err != nil {
			return fmt.Errorf("Error looking up system settlement account: %v", err)
		}
		destAcct, err := q.GetAccountByIDForUpdate(r.Context(), DestinationAccount.ID)
		if err != nil {
			return fmt.Errorf("Error looking up destination account: %v", err)
		}
		trf, err = q.CreateTransferRequest(r.Context(), db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempkey.ID,
			CustomerID:           customer.ID,
			SourceAccountID:      systemSettleAcct.ID,
			DestinationAccountID: destAcct.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			ClientReference:      ClientReference,
			ExternalReference:    ExternalReference,
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating transfer request for deposit: %v", err)
		}

		jtx, err = q.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempkey.ID,
			Status:            "pending",
			EntryType:         "deposit",
			AccountingDate:    pgtype.Date{Time: trf.RequestedAt.Time, Valid: true},
			EffectiveAt:       pgtype.Timestamptz{Time: trf.RequestedAt.Time, Valid: true},
			SourceSystem:      params.Sourcesystem,
			SourceEventID:     pgtype.Text{String: trf.ID.String(), Valid: true},
			Description:       pgtype.Text{String: params.Description, Valid: params.Description != ""},
			Metadata:          metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating journal transaction: %v", err)
		}
		legs := []JournalLeg{
			{AccountID: systemSettleAcct.ID, Amount: amount, Side: "debit"},
			{AccountID: destAcct.ID, Amount: amount, Side: "credit"},
		}
		err = ValidateLedgerBalance(legs)
		if err != nil {
			return err
		}
		var destLineID pgtype.UUID
		for index, leg := range legs {
			line, err := q.CreateJournalLine(r.Context(), db.CreateJournalLineParams{
				JournalTransactionID: jtx.ID,
				LineNumber:           int32(index) + 1,
				AccountID:            leg.AccountID,
				Side:                 leg.Side,
				Amount:               leg.Amount,
				CurrencyCode:         params.CurrencyCode,
				BalanceKind:          "available",
				Memo:                 memo,
				Metadata:             metaBytes,
			})
			if err != nil {
				return fmt.Errorf("Error creating journal line: %v", err)
			}
			if leg.AccountID == destAcct.ID && leg.Side == "credit" {
				destLineID = line.ID
			}
		}
		_, err = q.MarkJournalTransactionPosted(r.Context(), jtx.ID)
		if err != nil {
			return fmt.Errorf("Error Marking journal transaction: %v", err)
		}
		balance, err := GetOrCreateBalanceProjection(
			r.Context(), q,
			destAcct.ID,
			params.CurrencyCode,
			"available",
		)
		if err != nil {
			return fmt.Errorf("getting balance: %w", err)
		}
		destLedger, _ := decimal.NewFromString(NumericToString(balance.LedgerBalance))
		destHeld, _ := decimal.NewFromString(NumericToString(balance.HeldBalance))
		amt, _ := decimal.NewFromString(NumericToString(amount))
		newDestLedger := destLedger.Add(amt)
		newDestHeld := destHeld
		newDestAvail := newDestLedger.Sub(newDestHeld)

		newDestLedgerNum, err := StringToNumeric(newDestLedger.String())
		newDestAvailNum, err := StringToNumeric(newDestAvail.String())
		newDestHeldNum, err := StringToNumeric(newDestHeld.String())
		err = q.UpsertBalanceProjectionWithExpectedVersion(r.Context(), db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        destAcct.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    newDestLedgerNum,
			AvailableBalance: newDestAvailNum,
			HeldBalance:      newDestHeldNum,
			LastTxID:         jtx.ID,
			LastLineID:       destLineID,
			ExpectedVersion:  balance.Version,
		})
		if err != nil {
			return fmt.Errorf("upserting destination balance: %w", err)
		}
		trf, err = q.UpdateTransferRequestStatus(r.Context(), db.UpdateTransferRequestStatusParams{
			Status: "posted",
			ID:     trf.ID,
		})
		if err != nil {
			return fmt.Errorf("Error Updating the transfer reuest status : %w", err)
		}
		transferPayload := ToTransferResponse(trf, &jtx)
		payloadBytes, err = json.Marshal(transferPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox payload: %w", err)
		}
		headersMap := map[string]interface{}{
			"content_type":      "application/json",
			"source_system":     params.Sourcesystem,
			"event_transaction": jtx.TransactionRef,
			"client_reference":  params.ClientReference,
		}
		headersBytes, err := json.Marshal(headersMap)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox headers: %w", err)
		}
		partitionKey := pgtype.Text{String: trf.ID.String(), Valid: true}
		_, err = q.CreateOutboxEvent(r.Context(), db.CreateOutboxEventParams{
			AggregateType:    "transfer_request",
			AggregateID:      trf.ID,
			EventType:        "transfer.posted",
			IdempotencyKeyID: idempkey.ID,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     partitionKey,
		})
		if err != nil {
			return fmt.Errorf("failed to create outbox event: %w", err)
		}
		if err := api.saveIdem(r.Context(), params.CustomerID, params.IdempotencyKeyID, idempkey.ID, payloadBytes, 201); err != nil {
			return fmt.Errorf("error saving idempotency key: %v", err)
		}
		return nil
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("transfer failed: %v", err))
		return
	}
	respondeWithJson(w, 201, ToTransferResponse(trf, &jtx))
}

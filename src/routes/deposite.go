package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
	requestHash := hashRequest(bodyBytes)

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := DepositeParams{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.SourceAccountID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Description == "" {
		respondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, description")
		return
	}
	customerID, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing customer ID: %v", err))
		return
	}
	idempotencyKey, err := StringtoPgUuid(params.IdempotencyKeyID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Idempotency KeyID: %v", err))
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
	var trf db.TransferRequest
	err = api.Db.ExecTx(r.Context(), func(q *db.Queries) error {
		check, err := api.IdemCheck(r.Context(), params.IdempotencyKeyID, params.CustomerID, requestHash, "deposite_create")
		if err != nil {
			return fmt.Errorf("Error checking idempotency key: %v", err)
		}
		if !check.ShouldProceed {
			return fmt.Errorf("idem response %v, status code %v ", check.CachedResponse, check.StatusCode)
		}
		destAcct, err := q.GetAccountByIDForUpdate(r.Context(), DestinationAccountID)
		if err != nil {
			return fmt.Errorf("Error looking up source account: %v", err)
		}
		balance, err := q.GetBalanceProjectionForUpdate(r.Context(), db.GetBalanceProjectionForUpdateParams{
			AccountID:    destAcct.ID,
			CurrencyCode: destAcct.CurrencyCode,
			BalanceKind:  "available",
		})
		settlementAcct, err := q.GetAccountByExternalRef(r.Context(), fmt.Sprintf("settlement-%s", params.CurrencyCode))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("settlement account not found for currency: %s", params.CurrencyCode)
			}
			return fmt.Errorf("Error looking up settlement account: %v", err)
		}
		trf, err := q.CreateTransferRequest(r.Context(), db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempotencyKey,
			CustomerID:           customer.ID,
			SourceAccountID:      settlementAcct.ID,
			DestinationAccountID: DestinationAccount.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			ClientReference:      ClientReference,
			ExternalReference:    ExternalReference,
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating transfer request: %v", err)
		}

		jtx, err := q.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempotencyKey,
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
			{AccountID: settlementAcct.ID, Amount: amount, Side: "debit"},
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
		destLedger, _ := decimal.NewFromString(NumericToString(balance.LedgerBalance))
		destHeld, _ := decimal.NewFromString(NumericToString(balance.HeldBalance))
		amt, _ := decimal.NewFromString(NumericToString(amount))
		newDestLedger := destLedger.Add(amt)
		newDestHeld := destHeld
		newDestAvail := newDestLedger.Add(destHeld)

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
			Status: "Posted",
			ID:     trf.ID,
		})
		if err != nil {
			return fmt.Errorf("Error Updating the transfer reuest status : %w", err)
		}
		payloadBytes, err := json.Marshal(trf)
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
			IdempotencyKeyID: idempotencyKey,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     partitionKey,
		})
		if err != nil {
			return fmt.Errorf("failed to create outbox event: %w", err)
		}

		if err := api.saveIdem(r.Context(), params.IdempotencyKeyID, "transfer_create", params.CustomerID, requestHash, payloadBytes, 201); err != nil {
			return fmt.Errorf("error saving idempotency key: %w", err)
		}
		return nil
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("transfer failed: %v", err))
		return
	}
	respondeWithJson(w, 201, trf)
	return
}

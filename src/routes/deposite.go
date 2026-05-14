package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type DepositeParams struct {
	Provider             string                 `json:"provider"`
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

// HandleDeposite credits funds to an account from the system settlement account.
// @Summary Deposit funds to account
// @Description Credits funds to an account from the system settlement account. Supports idempotent deposits.
// @Tags Deposits
// @Accept json
// @Produce json
// @Param body body DepositeParams true "Deposit Details"
// @Success 202 {object} map[string]interface{}{"success":true,"status":"pending","reference":"DEP-{idempotency_key_id}","message":"Transfer is being processed. You will be notified via webhook."}
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /account/deposite [post]
func (api *ApiConfig) HandleDeposite(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := DepositeParams{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.Provider == "" || params.IdempotencyKeyID == "" || params.CustomerID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Sourcesystem == "" || params.Amount == "" || params.FeeAmount == "" || params.ClientReference == "" || params.ExternalReference == "" {
		respondWithError(w, 400, "missing required fields: provider, idempotency_key_id, customer_id, destination_account_id, currency_code, source_system, amount, fee_amount, client_reference, external_reference")
		return
	}
	if params.Metadata == nil {
		params.Metadata = make(map[string]interface{})
	}
	payload, err := json.Marshal(params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	topic := "deposite.transfer"
	if err := api.Kafka_producer.SendMessage(topic, params.CustomerID, payload); err != nil {
		log.Printf("Kafka Failure: %v", err)
		respondWithError(w, 500, "internal storage error")
		return
	}
	respondeWithJson(w, 202, map[string]interface{}{
		"success":   true,
		"status":    "pending",
		"reference": fmt.Sprintf("DEP-%s", params.IdempotencyKeyID),
		"message":   "Transfer is being processed. You will be notified via webhook.",
	})
}

func DepositeLogic(ctx context.Context, payload []byte, api *ApiConfig) (*TransferResponse, error) {
	var params DepositeParams
	if err := json.Unmarshal(payload, &params); err != nil {
		log.Printf("Error parsing payload: %v", err)
		return nil, err
	}
	requestHash := HashRequest(payload)
	customerID, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("Error parsing customer ID: %v", err)
	}
	customer, err := api.Db.Queries.GetCustomerByID(ctx, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("customer not found")
		}
		return nil, fmt.Errorf("Error looking up customer: %v", err)
	}
	if customer.Status != "active" {
		return nil, fmt.Errorf("customer is not active")
	}
	amount, err := StringToNumeric(params.Amount)
	feeAmount, err := StringToNumeric(params.FeeAmount)
	ClientReference := pgtype.Text{String: params.ClientReference, Valid: params.ClientReference != ""}
	ExternalReference := pgtype.Text{String: params.ExternalReference, Valid: params.ExternalReference != ""}
	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			return nil, fmt.Errorf("Error parsing metadata: %v", err)
		}
		metaBytes = b
	}
	memo := pgtype.Text{String: params.Memo, Valid: params.Memo != ""}
	check, err := api.IdemCheck(ctx, params.IdempotencyKeyID, params.CustomerID, requestHash, "deposit_create")
	if err != nil {
		return nil, fmt.Errorf("Error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		var cachedData TransferResponse
		err := json.Unmarshal(check.CachedResponse, &cachedData)
		if err != nil {
			fmt.Printf("Failed to unmarshal cached response: %v", err)
			return nil, fmt.Errorf("Failed to unmarshal cached response: %v", err)
		}
		return &cachedData, nil
	}
	idempkey, err := api.Db.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKeyID,
		Scope:          "deposit_create",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating idempotency key: %v", err)
	}

	var trf db.TransferRequest
	var jtx db.JournalTransaction
	var payloadBytes []byte
	err = api.Db.ExecTx(ctx, func(q *db.Queries) error {
		// Optimization: Fetch FOR UPDATE directly inside the TX using the reference/ID
		// This saves two round trips to the DB.
		systemSettleAcct, err := q.GetAccountByExternalRef(ctx, "system_settlement_ngn")
		if err != nil {
			return fmt.Errorf("Error looking up system settlement account: %v", err)
		}

		destUuid, _ := StringtoPgUuid(params.DestinationAccountID)
		destAcct, err := q.GetAccountByIDForUpdate(ctx, destUuid)
		if err != nil {
			return fmt.Errorf("Error looking up destination account: %v", err)
		}

		if destAcct.Status != "active" {
			return fmt.Errorf("destination account is not active")
		}
		if params.CurrencyCode != destAcct.CurrencyCode {
			return fmt.Errorf("currency code mismatch")
		}

		trf, err = q.CreateTransferRequest(ctx, db.CreateTransferRequestParams{
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

		jtx, err = q.CreateJournalTransaction(ctx, db.CreateJournalTransactionParams{
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
			line, err := q.CreateJournalLine(ctx, db.CreateJournalLineParams{
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
		_, err = q.MarkJournalTransactionPosted(ctx, jtx.ID)
		if err != nil {
			return fmt.Errorf("Error Marking journal transaction: %v", err)
		}
		balance, err := GetOrCreateBalanceProjection(
			ctx, q,
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
		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
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
		trf, err = q.UpdateTransferRequestStatus(ctx, db.UpdateTransferRequestStatusParams{
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
		partitionKey := pgtype.Text{String: trf.DestinationAccountID.String(), Valid: true}
		_, err = q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			AggregateType:    "deposit_request",
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
		if err := api.saveIdem(ctx, params.CustomerID, params.IdempotencyKeyID, idempkey.ID, payloadBytes, 201); err != nil {
			return fmt.Errorf("error saving idempotency key: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("transfer failed: %v", err)
	}
	response := ToTransferResponse(trf, &jtx)
	return &response, nil

}
